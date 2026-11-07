#!/bin/bash
# Build a client-only device IPA for the owner's Feather signing workflow.
set -euo pipefail
TASK_IOS_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$TASK_IOS_ROOT"
[[ -f APTEX/Info.plist && -f APTEX/Resources/Assets.xcassets/AppIcon.appiconset/AppIcon.png ]] || { echo 'APTEX iOS sources or icon are missing.' >&2; exit 1; }
TASK_SOURCE_REVISION=${APTEX_SOURCE_REVISION:-}
if [[ -z "$TASK_SOURCE_REVISION" ]]; then
  TASK_SOURCE_REVISION=$(git -C "$TASK_IOS_ROOT" rev-parse HEAD 2>/dev/null) || { echo 'Set APTEX_SOURCE_REVISION when building an exported source tree.' >&2; exit 1; }
  [[ -z "$(git -C "$TASK_IOS_ROOT" status --porcelain -- .)" ]] || { echo 'Commit the iOS sources before packaging a revision-tagged release.' >&2; exit 1; }
fi
[[ "$TASK_SOURCE_REVISION" =~ ^[0-9a-f]{40}$ ]] || { echo 'APTEX_SOURCE_REVISION must be the full source commit SHA.' >&2; exit 1; }
TASK_OUTPUT=${APTEX_OUTPUT_DIR:-"$TASK_IOS_ROOT/build/release"}
mkdir -p "$TASK_OUTPUT"
TASK_OUTPUT=$(cd "$TASK_OUTPUT" && pwd)
python3 scripts/check-localizations.py
python3 scripts/generate_project.py
xcodebuild -project APTEX.xcodeproj -scheme APTEX -configuration Release \
  -destination 'generic/platform=iOS' -derivedDataPath device-build \
  APTEX_SOURCE_REVISION="$TASK_SOURCE_REVISION" CODE_SIGNING_ALLOWED=NO build
TASK_PACKAGE=$(mktemp -d "${TMPDIR:-/tmp}/aptex-ipa.XXXXXX")
trap 'rm -rf "$TASK_PACKAGE"' EXIT
mkdir -p "$TASK_PACKAGE/Payload"
ditto --norsrc device-build/Build/Products/Release-iphoneos/APTEX.app "$TASK_PACKAGE/Payload/APTEX.app"
TASK_APP="$TASK_PACKAGE/Payload/APTEX.app"
if [[ -d "$TASK_APP/Frameworks" ]]; then
  while IFS= read -r -d '' binary; do codesign --force --sign - --timestamp=none "$binary"; done < <(find "$TASK_APP/Frameworks" -type f -name '*.dylib' -print0)
  while IFS= read -r -d '' framework; do codesign --force --sign - --timestamp=none "$framework"; done < <(find "$TASK_APP/Frameworks" -type d -name '*.framework' -print0)
fi
codesign --force --sign - --timestamp=none --generate-entitlement-der "$TASK_APP"
codesign --verify --deep --strict "$TASK_APP"
python3 - "$TASK_APP" "$TASK_SOURCE_REVISION" <<'PY'
import json,pathlib,plistlib,subprocess,sys
app=pathlib.Path(sys.argv[1]);info=plistlib.loads((app/'Info.plist').read_bytes())
assert info['CFBundleIdentifier']=='com.afterglowlabs.aptex'
assert info['APTEXSourceRevision']==sys.argv[2]
assert subprocess.check_output(['lipo','-archs',str(app/'APTEX')],text=True).strip()=='arm64'
assert not list(app.rglob('*.xctest')) and not (app/'embedded.mobileprovision').exists()
assert b'fixture-aptex-1234' not in (app/'APTEX').read_bytes(), 'Test password was included in release code'
print(json.dumps({'bundle_id':info['CFBundleIdentifier'],'version':info['CFBundleShortVersionString'],'build':info['CFBundleVersion'],'source':info['APTEXSourceRevision'],'architecture':'arm64','signing':'ad-hoc; re-sign with Feather'}))
PY
ditto --norsrc -c -k --keepParent "$TASK_PACKAGE/Payload" "$TASK_OUTPUT/APTEX-iOS-Feather.ipa"
(cd "$TASK_OUTPUT" && shasum -a 256 APTEX-iOS-Feather.ipa > SHA256SUMS.txt)
printf 'IPA: %s/APTEX-iOS-Feather.ipa\n' "$TASK_OUTPUT"
