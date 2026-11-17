# Analysis: `permission.go`

A small, self-contained permission resolver: ordered rules → last match wins → default `ask`. The core logic is sound in shape, but there are several real bugs and a security-relevant gap.

## Correctness bugs

### 1. `**` does not match zero path segments (biggest functional bug)

`strings.ReplaceAll(pattern, "**", "*")` turns `a/**/b` into `a/*/b`, which requires at least one segment between. So `a/**/b` fails to match `a/b`. Same for `a/**` vs `a`. If `**` is meant to be "any sequence including `/`", it must also match empty.

### 2. `matchStar` is byte-based, not rune-based

`?` compares single bytes, so `Match("?", "é")` is `false` (é is 2 bytes). Any non-ASCII resource path breaks. Use `[]rune` or `utf8.DecodeRuneInString`.

### 3. Inconsistent semantics between the two match paths

`filepath.Match` supports `[abc]` classes; `matchStar` treats `[` literally. Worse, a malformed pattern (`[`) makes `filepath.Match` return an error and `Match` returns `false` immediately — it never reaches the `matchStar` fallback. So the fallback only runs for *valid* patterns that didn't match, which is a confusing split.

### 4. Dead code in `Resolve`

`matched` is assigned and never read (`_ = matched`). Leftover from an earlier design; delete it.

### 5. `Effect` is an unvalidated string

A typo (`"alow"`) or a bad YAML value flows straight through `Resolve` to the caller. Depending on how the caller compares, an invalid effect silently degrades to deny or ask. Make it a typed enum and validate at load time.

### 6. `Assertion.ToolName` / `ToolCallID` are unused

Either dead fields or a missing feature (tool-scoped rules). Worth confirming intent — if rules should be able to key on tool name, `Resolve` currently ignores it.

## Security gap

**No path normalization.** `Match` is used for resources, and `*` deliberately crosses `/`. So a rule like `allow read /home/user/*` matches `/home/user/../../etc/passwd`. There's no `filepath.Clean` on the resource, no symlink resolution, and no rejection of `..` segments. For a permission gate this is the thing to fix first: clean the resource before matching, and reject any resource whose cleaned form escapes the rule's root.

Also worth deciding explicitly: **overlays always win**, including `deny` in agent rules being overridden by `allow` in an overlay. That's probably intended (user overrides), but there's no way to express "deny is sticky."

## Performance / structure

- `Resolve` allocates a combined slice on every call. Iterate `agentRules` then `overlays` directly — zero alloc.
- Patterns are re-parsed on every assertion (`filepath.Match` twice per rule). If rule sets are non-trivial, precompile rules into a matcher at load time.
- No tests in the file. The `**` and UTF-8 bugs would both be caught by a table test.

## Suggested minimal fixes

```go
// 1. drop the dead variable
effect := EffectAsk
for _, r := range agentRules {
    if Match(r.Action, a.Action) && Match(r.Resource, a.Resource) {
        effect = r.Effect
    }
}
for _, r := range overlays {
    if Match(r.Action, a.Action) && Match(r.Resource, a.Resource) {
        effect = r.Effect
    }
}
return effect
```

```go
// 2. make ** match zero segments, and drop the filepath.Match detour
func Match(pattern, value string) bool {
    if pattern == "" {
        return value == ""
    }
    return matchStar(pattern, []rune(value))
}
// in matchStar: on '*', if next rune is also '*', consume it and allow
// the star to match zero runes (i.e. don't force match++ before retrying).
```

```go
// 3. normalize resources before matching
a.Resource = filepath.Clean(a.Resource)
if strings.Contains(a.Resource, "..") {
    return EffectDeny
}
```

## Next step

Write the corrected version plus a table test covering `a/**/b` vs `a/b`, UTF-8, and `..` traversal.
