import json, re, subprocess
from pathlib import Path

SATURN = 'G:/workspace/projects/saturn'
ZENBOT = Path('G:/workspace/projects/zenbot-saturn-migration')

def git_show(path):
    return subprocess.check_output(['git','-C',SATURN,'show','origin/develop:'+path], text=True)

def git_files(prefix):
    out = subprocess.check_output(['git','-C',SATURN,'ls-tree','-r','--name-only','origin/develop',prefix], text=True)
    return [x for x in out.splitlines() if x.endswith('.java')]

source=[]
for f in git_files('src/main/java/org/saturn/app/command/impl'):
    text=git_show(f)
    m=re.search(r'@CommandAliases\s*\(\s*aliases\s*=\s*\{([^}]*)\}',text,re.S)
    if not m: continue
    aliases=re.findall(r'"([^"]+)"',m.group(1))
    if not aliases: continue
    source.append({'file':f,'class':Path(f).stem,'aliases':aliases,'canonical':aliases[0]})

registry=(ZENBOT/'internal/command/registry.go').read_text()
handlers=(ZENBOT/'internal/command/handlers.go').read_text()
dispatch=(ZENBOT/'internal/command/dispatch_adapter.go').read_text()
# Extract canonical definitions from catalog: def("canonical", []string{aliases...}
entries={}
for m in re.finditer(r'def\("([^"]+)",\s*\[\]string\{([^}]*)\}',registry):
    entries[m.group(1)]=re.findall(r'"([^"]+)"',m.group(2))
# concrete newCommand cases; individual switch cases can contain multiple literals
concrete=set()
for m in re.finditer(r'case\s+([^:]+):\s*\n\s*return\s+&',handlers):
    concrete.update(re.findall(r'"([^"]+)"',m.group(1)))
# Explicit registrations before/after dynamic conditions
registered=set(re.findall(r'"([^"]+)"',dispatch))
rows=[]
for item in source:
    canonical=item['canonical']
    catalog_aliases=entries.get(canonical,[])
    missing_aliases=sorted(set(item['aliases'])-set(catalog_aliases))
    status='CONCRETE' if canonical in concrete else 'PLACEHOLDER_OR_EXTERNAL'
    # registered strings include comments and unrelated, only a best-effort static signal.
    registered_hint=canonical in registered
    rows.append({**item,'catalog_aliases':catalog_aliases,'missing_aliases':missing_aliases,'dispatch':status,'registration_hint':registered_hint})

rows.sort(key=lambda r:(r['file'],r['canonical']))
summary={
 'source_command_classes':len(rows),
 'source_aliases':sum(len(r['aliases']) for r in rows),
 'catalog_alias_mismatches':sum(bool(r['missing_aliases']) for r in rows),
 'concrete_dispatch':sum(r['dispatch']=='CONCRETE' for r in rows),
 'placeholder_or_external':sum(r['dispatch']!='CONCRETE' for r in rows),
}
out={'summary':summary,'commands':rows}
(ZENBOT/'.hermes'/'command-crosscheck.json').write_text(json.dumps(out,indent=2)+'\n')
lines=['# Saturn → Zenbot command cross-check','','Generated from Saturn `origin/develop` command annotations and Zenbot’s static catalog/dispatch code. `PLACEHOLDER_OR_EXTERNAL` requires source-level review; it is not migration acceptance.','',f"- Source command classes: **{summary['source_command_classes']}**",f"- Source aliases: **{summary['source_aliases']}**",f"- Catalog alias mismatches: **{summary['catalog_alias_mismatches']}**",f"- Concrete dispatch cases: **{summary['concrete_dispatch']}**",f"- Placeholder/external cases: **{summary['placeholder_or_external']}**",'','| Source class | Canonical | Saturn aliases | Catalog aliases | Dispatch | Alias gap |','|---|---|---|---|---|---|']
for r in rows:
    lines.append('| `'+r['class']+'` | `'+r['canonical']+'` | '+', '.join('`'+x+'`' for x in r['aliases'])+' | '+', '.join('`'+x+'`' for x in r['catalog_aliases'])+' | '+r['dispatch']+' | '+(', '.join(r['missing_aliases']) or '—')+' |')
(ZENBOT/'.hermes'/'command-crosscheck.md').write_text('\n'.join(lines)+'\n')
print(json.dumps(summary))
