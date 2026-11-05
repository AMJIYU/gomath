# Subtitle fixtures

Hand-authored UTF-8 fixtures; no production encoder generated the base files.

- `multiline.srt`: nonsequential retained indices, Chinese/emoji, literal `&`,
  multiline text, preserved spaces, adjacent cue boundaries and hour timestamps.
- `multiline-bom-crlf.srt`: the same bytes with initial UTF-8 BOM and CRLF endings.
- `plain.vtt`: optional identifiers, short/full timestamps, all six supported
  WebVTT entities and escaped angle brackets that remain literal text.
- `unsupported-markup.vtt`: visible class markup that must be rejected.
- `unsupported-position.vtt`: positioning settings that must be rejected.

Expected times and text are independent constants in the central Go tests.
Ordinary tests use only Go's standard library and these local files.

The centralized subtitle TestMain also uses this directory as the test/fuzz
working directory. Go's failure corpus is therefore stored under
`testdata/fuzz/<FuzzTarget>/` here, never in production `subtitle/testdata/`.
