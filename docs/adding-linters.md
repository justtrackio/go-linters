# Add a linter

1. Implement the analyzer with the existing `go/analysis` plugin pattern.
   Add it to `JustTrackPlugin.BuildAnalyzers` in `justtrack.go`.
   Keep the aggregate load mode sufficient for every analyzer.

2. Add Go fixtures under `testdata/src/testlintdata/<rule>/`.
   Mark expected diagnostics with `// want "diagnostic pattern"`.
   Include valid code, boundary cases, and false-positive regressions.

3. Add `<rule>_test.go` using Testify `require` and `analysistest.Run`.
   Reuse `testdataDir(t)` and pass the fixture package paths.
   Test behavior, not plugin counts or diagnostic wording.

4. If the rule offers fixes, use `analysistest.RunWithSuggestedFixes` instead.
   Add an adjacent `<file>.go.golden` with the complete expected Go source.
   Check imports, variable scope, and unchanged valid code in the expected output.
   Review failures before changing a golden file.

5. Run the checks:

   ```sh
   mise run test
   mise run bundle
   mise run test-bundle
   ```

   The bundle test copies existing fixtures into a temporary workspace.
   It discovers `.go.golden` files, runs real `--fix`, compares output, and compiles the corrected packages.
   It also checks that the corrected packages lint cleanly.
   Do not add duplicate examples or a separate bundle case for each rule.

6. Merge the change and publish a new `v*` tag.
   Consumers enable `justtrack` once and update the bundle version or refresh `latest`.
   No new lint command or per-rule consumer configuration is required.
