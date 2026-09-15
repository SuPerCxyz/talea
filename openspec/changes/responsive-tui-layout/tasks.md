## 1. Session list layout

- [x] 1.1 Remove the fixed 100-character pre-truncation from session question and latest-message descriptions while preserving fixed metadata widths.
- [x] 1.2 Add regression coverage showing wide-terminal question text growth, narrow-terminal safe clipping, CJK display-width handling, and resize-safe list interaction.

## 2. Detail table layout

- [x] 2.1 Make model summary, user-turn, and sub-agent rows allocate remaining viewport width to variable text columns and truncate safely when narrow.
- [x] 2.2 Add regression coverage for wide and narrow detail rows and content regeneration after viewport resize.

## 3. Verification

- [x] 3.1 Run gofmt and focused TUI tests, then run go vet and the full Go test suite.
- [x] 3.2 Validate the OpenSpec change and inspect the final diff for scope, formatting, and unrelated artifacts.
