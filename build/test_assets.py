"""Regression guard for the scoped daisyUI join stacking rules.

Tailwind 4.3.4/4.3.5 dropped daisyUI 5.7.x relative focus/hover
selectors while hoisting @scope. Build CSS before running this check.
"""

from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]


class GeneratedStylesheetTests(unittest.TestCase):
    def test_join_keeps_focus_and_hover_stacking(self):
        css = (ROOT / "static/css/app.css").read_text()
        scope = re.search(r"@scope\(\.join\)", css)
        self.assertIsNotNone(scope, "join's scoped component rules must exist")
        assert scope is not None
        # Isolate the scope by balanced braces, not by the next component's name.
        start = css.index("{", scope.end())
        end = start + 1
        depth = 1
        while depth and end < len(css):
            depth += (css[end] == "{") - (css[end] == "}")
            end += 1
        self.assertEqual(depth, 0, "join scope must have balanced braces")
        join = css[start + 1:end - 1]
        self.assertRegex(join, r":where\([^{}]*:focus[^{}]*\)\{z-index:2\}")
        self.assertRegex(join, r":where\([^{}]*\.btn:hover[^{}]*\)\{z-index:1\}")


if __name__ == "__main__":
    unittest.main()
