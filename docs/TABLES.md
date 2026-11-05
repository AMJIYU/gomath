# Editable vector tables

`table.New` lays out strings with explicit `text.Options`/`Font`;
`table.NewObjects` accepts formulae, images and arbitrary valid object groups.
Inputs are copied independently for every cell, including repeated pointers.
Column widths and row heights come from maximum ink bounds, explicit minima
and padding. Layout runs once at construction; later content changes do not
automatically resize cells or reflow the table.

## Headers and indexing

`Options.RowLabels` adds a left column; `ColumnLabels` adds a top row. A nonempty
slice must have exactly the number of data rows/columns, with valid non-nil
objects. `Corner` requires both sets; absent corner content leaves an owned
empty group. The corner still occupies its full grid cell and background.
All label objects are copied with new IDs and placed by the same layout as data.

```go
options := table.DefaultOptions()
options.RowLabels = []*gomath.Object{rowA, rowB}
options.ColumnLabels = []*gomath.Object{columnA, columnB}
options.Corner = heading
view, err := table.NewObjects(data, options)
if err != nil { return err }
entry, err := view.Entry(0, 0) // first data entry, independent of headers
if err != nil { return err }
if err := scene.Add(view.Object); err != nil { return err }
return scene.Play(1, gomath.Transform(entry, entry.Copy().SetColor(gomath.Blue)))
```

| API | Coordinates / result |
| --- | --- |
| `Dimensions`, `Entry`, `Row`, `Column`, `CellBackground`, `HighlightCell` | Zero-based data coordinates; headers excluded. Existing indices do not shift when labels are added. |
| `GridDimensions`, `GridEntry`, `GridCellBackground` | Zero-based complete layout, including header row/column and optional empty corner. |
| `RowLabels`, `ColumnLabels` | Fresh slices holding the actual owned label references. |
| `Corner` | Actual owned explicit corner, or nil when absent. |
| `Labels` | Fresh selection ordered explicit corner, column labels, row labels. The blank corner is excluded. |
| `Lines` | Actual group of rules, including header separators when inner lines are enabled. |

Selections are borrowed references, not duplicate parents to add alongside the
table. Whole-table transformations preserve data/header/background references;
replacing its child topology invalidates these semantic selections. Headers
can be animated independently like data cells. Background objects default to
transparent fill/stroke; direct background access can style headers as well as
data. No labels are inferred from the first data row.

## Bounds and validation

`MaxLayoutCells=65536` limits the full grid, including headers and blank corner,
before text construction or object copies. Geometry, fonts and scene/raster
budgets apply separately. All spacing/minima must be finite nonnegative values;
enabled rules require finite positive width and valid color. Invalid label
counts, missing objects, orphan corners and oversized layouts return errors and
do not mutate inputs. Input object/font authoring must finish before calls read
it; copied tables participate in normal scene snapshot/concurrency semantics.

## Independent reference

`tests/reference/table_labels.py` pins Manim CE0.22.1 and commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`. The JSON fixture in
`tests/data/table/manim-labels-0.22.1.json` contains 15 static configurations:
no headers, rows only, columns only, both with blank corner, and explicit corner;
each is checked before and after two rigid transforms. Rectangular ink entries
avoid font differences. Central Go tests compare every visible entry center and
bounding size plus all rule endpoints at absolute tolerance 1e-11. Generator
hash, source version and source commit are enforced.

This verifies bounded table geometry with selected spacing and zero cell minima,
not all Manim Table defaults, fonts, animation timing or API semantics. GoMath
uses zero-based data access; Manim's APIs expose different one-based selections.
Merged cells, labels added after construction, automatic reflow, baseline/corner
alignment modes and code highlighting remain unimplemented.

Run `demo -scene table-labels` for editable headers and a data-cell highlight.
All table tests, reference scripts and fixtures remain under `tests/`.
