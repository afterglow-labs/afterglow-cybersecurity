#!/usr/bin/env python3
"""Check the native UI's literal labels and every shipped language entry."""
import json,re
from pathlib import Path
root=Path(__file__).resolve().parents[1]
resources=root/'APTEX/Resources'
required={'en','ko','zh-Hans','zh-Hant','ja','es'}
for file in resources.glob('*.xcstrings'):
    catalog=json.loads(file.read_text())
    assert catalog['sourceLanguage']=='en', file
    for key,entry in catalog['strings'].items():
        assert set(entry['localizations'])==required,(file.name,key,'missing language')
        for locale,value in entry['localizations'].items():
            unit=value['stringUnit']
            assert unit['state']=='translated' and unit['value'].strip(),(file.name,key,locale)
labels=set()
for file in (root/'APTEX').rglob('*.swift'):
    for pattern in [r'(?:Text|Button|Label|Section|Picker|TextField|SecureField|ProgressView|ContentUnavailableView)\("([^"\\]*(?:\\.[^"\\]*)*)"',r'\.(?:clientNavigationTitle|navigationTitle)\("([^"\\]*(?:\\.[^"\\]*)*)"',r'\.accessibilityLabel\("([^"\\]*(?:\\.[^"\\]*)*)"',r'prompt: "([^"\\]*(?:\\.[^"\\]*)*)"']:
        labels.update(value for value in re.findall(pattern,file.read_text()) if value and '\\(' not in value)
strings=json.loads((resources/'Localizable.xcstrings').read_text())['strings']
missing=sorted(labels-set(strings))
assert not missing, 'Untranslated literal UI labels: '+repr(missing)
print(f'English-primary catalogs verified: {len(strings)} labels, six languages, localized network permission.')
