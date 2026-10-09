# MVP Tweaks for Oct 8th 2026

## Revision 1
### Fix 1 - No packs have translated files.
* No packs have translate source data ever. The only thing a pack can be is refer to the online file / skill / plugin. In each pack directory there can be only one file manifest.toml.

### Fix 2 - Fix cache Folder structure.
* Format: `<githuibusername>/<reponame>/<commit sha or tag or release or main or whatever is logical?>/files`
* In manfiest you can tag which version you want otherwise it defaults to latest main.
* Use conventions from npm to decide on how to style this.

### Fix 3 - Ignore hooks and MCPs
* just focus on skills and mcps for now.


