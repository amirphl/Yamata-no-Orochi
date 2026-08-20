import importlib.util
import random
import sys
import unittest
from pathlib import Path

SCRIPT_PATH = Path(__file__).with_name("generate-synthetic-audience-profiles.py")
sys.path.insert(0, str(SCRIPT_PATH.parent))
SPEC = importlib.util.spec_from_file_location("synthetic_profiles", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class SyntheticAudienceProfileTests(unittest.TestCase):
    def test_generation_has_sequential_phones_and_distinct_tags(self) -> None:
        rows = list(
            MODULE.generate_profiles(
                start_offset=0,
                count=3,
                phone_start=989_010_000_000,
                uid_prefix="syn_",
                tag_ids=list(range(100, 150)),
                tags_per_profile=20,
                colors=["white", "pink", "black"],
                rng=random.Random(7),
            )
        )

        self.assertEqual(
            [row.phone_number for row in rows],
            ["989010000000", "989010000001", "989010000002"],
        )
        self.assertEqual(len({row.uid for row in rows}), 3)
        for row in rows:
            self.assertEqual(len(row.phone_number), 12)
            self.assertTrue(row.phone_number.startswith("989"))
            self.assertEqual(len(row.tags), 20)
            self.assertEqual(len(set(row.tags)), 20)
            self.assertIn(row.color, {"white", "pink", "black"})
            self.assertGreaterEqual(row.normalized_score, 0.0)
            self.assertLess(row.normalized_score, 1.0)

    def test_copy_buffer_uses_postgres_array_syntax(self) -> None:
        profile = MODULE.SyntheticProfile(
            uid="syn_989010000000_abcdef123456",
            phone_number="989010000000",
            tags=(14502, 14522, 14641),
            color="white",
            normalized_score=0.5,
        )

        value = MODULE.profiles_to_copy_buffer([profile]).read()

        self.assertEqual(
            value,
            "syn_989010000000_abcdef123456\t989010000000\t"
            "{14502,14522,14641}\twhite\t0.5\n",
        )


if __name__ == "__main__":
    unittest.main()
