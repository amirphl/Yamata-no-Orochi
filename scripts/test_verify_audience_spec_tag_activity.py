import unittest

from verify_audience_spec_tag_activity import (
    VerificationError,
    build_verification_report,
    extract_redis_tag_locations,
)

TEST_TAG_ID = 13358


def leaf(tags, available=501):
    value = {
        "tags": tags,
        "available_audience": available,
        "white_users": available,
    }
    for field in (
        "distinct_users", "black_users", "pink_users", "weak_white",
        "good_white", "best_white", "weak_black", "good_black",
        "best_black", "weak_pink", "good_pink", "best_pink", "scored_users",
    ):
        value[field] = 0
    return value


class VerifyAudienceSpecTagActivityTests(unittest.TestCase):
    def test_reports_file_active_tags_missing_or_inactive_in_database(self):
        specs = {
            "sms": {
                "L1": {
                    "L2": {
                        "items": {
                            "L3": leaf(["1", "2", "3", str(TEST_TAG_ID)])
                        }
                    }
                }
            }
        }
        file_tags = {
            1: {"id": 1, "name": "one", "is_active": True},
            2: {"id": 2, "name": "two", "is_active": True},
            3: {"id": 3, "name": "three", "is_active": False},
        }
        database_tags = {
            1: {"id": 1, "name": "one", "is_active": True},
            2: {"id": 2, "name": "two", "is_active": False},
            3: {"id": 3, "name": "three", "is_active": True},
            TEST_TAG_ID: {
                "id": TEST_TAG_ID,
                "name": "test",
                "is_active": True,
            },
        }

        report = build_verification_report(
            specs,
            {"sms": "yamata::audience_spec:cache:v3:sms"},
            file_tags,
            database_tags,
        )

        self.assertFalse(report["passed"])
        self.assertEqual(
            [row["id"] for row in report["file_active_but_database_not_active"]],
            [2],
        )
        self.assertEqual(
            [row["id"] for row in report["redis_tags_not_active_in_database"]],
            [2],
        )
        self.assertEqual(
            [row["id"] for row in report["redis_tags_not_verified_active_by_file"]],
            [3, TEST_TAG_ID],
        )

    def test_required_test_tag_is_still_checked_against_database(self):
        specs = {
            "sms": {
                "L1-test": {
                    "L2-test": {
                        "items": {
                            "L3-test": leaf([str(TEST_TAG_ID)])
                        }
                    }
                }
            }
        }
        report = build_verification_report(
            specs,
            {"sms": "key"},
            {},
            {},
        )

        self.assertFalse(report["passed"])
        self.assertEqual(
            report["redis_tags_not_active_in_database"][0]["id"],
            TEST_TAG_ID,
        )
        self.assertEqual(
            report["redis_tags_not_active_in_database"][0]["database_status"],
            "missing",
        )

    def test_database_active_tag_passes_even_when_absent_from_file_export(self):
        specs = {
            "sms": {
                "L1-test": {
                    "L2-test": {
                        "items": {
                            "L3-test": leaf([str(TEST_TAG_ID)])
                        }
                    }
                }
            }
        }
        report = build_verification_report(
            specs,
            {"sms": "key"},
            {},
            {
                TEST_TAG_ID: {
                    "id": TEST_TAG_ID,
                    "name": "test",
                    "is_active": True,
                }
            },
        )

        self.assertTrue(report["passed"])
        self.assertEqual(report["redis_tags_not_active_in_database"], [])
        self.assertEqual(
            report["redis_tags_not_verified_active_by_file"][0]["id"],
            TEST_TAG_ID,
        )

    def test_invalid_redis_tag_value_fails(self):
        with self.assertRaisesRegex(VerificationError, "positive integer ID"):
            extract_redis_tag_locations(
                {
                    "L1": {
                        "L2": {
                            "items": {
                                "L3": leaf(["not-an-id"], 1)
                            }
                        }
                    }
                },
                "sms",
            )


if __name__ == "__main__":
    unittest.main()
