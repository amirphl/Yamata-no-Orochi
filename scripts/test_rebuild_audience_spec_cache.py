import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest import mock

from rebuild_audience_spec_cache import (
    REQUIRED_TEST_LEVELS,
    InputValidationError,
    build_audience_spec,
    redis_key,
    redis_lock_key,
    store_in_redis,
)


def stat(levels, white_users=10, calculated_at="2026-07-14 15:04:50 +00:00"):
    row = {
        "layer1_category": levels[0],
        "layer2_category": levels[1],
        "layer3_category": levels[2],
        "white_users": white_users,
        "calculated_at": calculated_at,
    }
    for field in (
        "distinct_users", "black_users", "pink_users", "weak_white",
        "good_white", "best_white", "weak_black", "good_black",
        "best_black", "weak_pink", "good_pink", "best_pink", "scored_users",
    ):
        row[field] = 0
    return row


def reference(tag_id, levels):
    return {
        "id": tag_id,
        "layer1_category": levels[0],
        "layer2_category": levels[1],
        "layer3_category": levels[2],
    }


class BuildAudienceSpecTests(unittest.TestCase):
    def test_only_active_tag_ids_are_emitted_as_sorted_strings(self):
        levels = ("one", "two", "three")
        spec, report = build_audience_spec(
            [stat(levels, white_users=42)],
            [reference(20, levels), reference(3, levels), reference(10, levels)],
            [
                {"id": 20, "is_active": True},
                {"id": 3, "is_active": True},
                {"id": 10, "is_active": False},
            ],
        )

        leaf = spec["one"]["two"]["items"]["three"]
        self.assertEqual(leaf["layer1_category"], "one")
        self.assertEqual(leaf["layer2_category"], "two")
        self.assertEqual(leaf["layer3_category"], "three")
        self.assertEqual(leaf["tags"], ["3", "20"])
        self.assertEqual(leaf["available_audience"], 42)
        self.assertEqual(leaf["white_users"], 42)
        self.assertEqual(report.tags_written, 2)
        self.assertEqual(report.active_references, 2)

    def test_latest_stats_snapshot_wins(self):
        levels = ("one", "two", "three")
        spec, _ = build_audience_spec(
            [
                stat(levels, 10, "2026-07-01 00:00:00 +00:00"),
                stat(levels, 99, "2026-07-02 00:00:00 +00:00"),
            ],
            [reference(1, levels)],
            [{"id": 1, "is_active": True}],
        )

        self.assertEqual(
            spec["one"]["two"]["items"]["three"]["available_audience"], 99
        )

    def test_stats_leaf_without_active_reference_is_reported_and_omitted(self):
        used = ("one", "two", "used")
        unused = ("one", "two", "unused")
        spec, report = build_audience_spec(
            [stat(used), stat(unused)],
            [reference(1, used)],
            [{"id": 1, "is_active": True}],
        )

        self.assertNotIn("unused", spec["one"]["two"]["items"])
        self.assertEqual(report.skipped_leaves_without_active_tags, (unused,))

    def test_reference_to_missing_tag_fails(self):
        levels = ("one", "two", "three")
        with self.assertRaisesRegex(InputValidationError, "absent from tags"):
            build_audience_spec([stat(levels)], [reference(1, levels)], [])

    def test_active_flag_must_be_json_boolean(self):
        levels = ("one", "two", "three")
        with self.assertRaisesRegex(InputValidationError, "JSON boolean"):
            build_audience_spec(
                [stat(levels)], [reference(1, levels)], [{"id": 1, "is_active": 1}]
            )

    def test_empty_active_result_is_rejected(self):
        levels = ("one", "two", "three")
        with self.assertRaisesRegex(InputValidationError, "empty audience spec"):
            build_audience_spec(
                [stat(levels)],
                [reference(1, levels)],
                [{"id": 1, "is_active": False}],
            )

    def test_redis_key_exactly_matches_go_prefix_behavior(self):
        self.assertEqual(redis_key("yamata:", "sms"), "yamata::audience_spec:cache:v4:sms")
        self.assertEqual(redis_key("", "sms"), "yamata:audience_spec:cache:v4:sms")
        self.assertEqual(
            redis_lock_key("yamata:", "sms"),
            "yamata::audience_spec:rebuild-lock:v4:sms",
        )

    def test_required_test_leaf_is_included_with_tag_17358(self):
        spec, report = build_audience_spec(
            [stat(REQUIRED_TEST_LEVELS, white_users=501)],
            [],
            [],
            include_required_test_leaf=True,
        )

        leaf = spec["L1-test"]["L2-test"]["items"]["L3-test"]
        self.assertEqual(leaf["tags"], ["17358"])
        self.assertEqual(leaf["available_audience"], 501)
        self.assertEqual(leaf["white_users"], 501)
        self.assertFalse(report.required_test_tag_verified_active)

    def test_required_test_leaf_rejects_capacity_of_500(self):
        with self.assertRaisesRegex(InputValidationError, "greater than 500"):
            build_audience_spec(
                [stat(REQUIRED_TEST_LEVELS, white_users=500)],
                [],
                [],
                include_required_test_leaf=True,
            )


class _FakeLock:
    def __init__(self):
        self.acquired = False
        self.released = False

    def acquire(self, blocking):
        self.acquired = blocking
        return True

    def release(self):
        self.released = True

    def reacquire(self):
        return self.acquired


class _FakeRedisClient:
    def __init__(self):
        self.values = {"yamata::audience_spec:cache:v4:sms": b'{"old":true}'}
        self.locks = {}
        self.closed = False
        self.expiries = {}

    def ping(self):
        return True

    def get(self, key):
        return self.values.get(key)

    def set(self, key, value, ex=None):
        self.values[key] = value
        self.expiries[key] = ex
        return True

    def pipeline(self, transaction=True):
        if transaction is not True:
            raise AssertionError("cache writes must use a Redis transaction")
        return _FakePipeline(self)

    def mget(self, keys):
        return [self.values.get(key) for key in keys]

    def lock(self, key, **_kwargs):
        lock = _FakeLock()
        self.locks[key] = lock
        return lock

    def close(self):
        self.closed = True


class _FakePipeline:
    def __init__(self, client):
        self.client = client
        self.commands = []

    def set(self, key, value, ex=None):
        self.commands.append((key, value, ex))
        return self

    def execute(self):
        return [self.client.set(key, value, ex=expiry) for key, value, expiry in self.commands]


class StoreInRedisTests(unittest.TestCase):
    def test_write_is_locked_backed_up_and_verified(self):
        client = _FakeRedisClient()
        fake_module = types.ModuleType("redis")
        fake_module.Redis = types.SimpleNamespace(
            from_url=mock.Mock(return_value=client)
        )
        fake_module.exceptions = types.SimpleNamespace(
            LockError=RuntimeError,
            RedisError=RuntimeError,
        )

        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.dict(sys.modules, {"redis": fake_module}):
                keys = store_in_redis(
                    redis_url="redis://example.invalid:6379",
                    redis_db=2,
                    prefix="yamata:",
                    payloads={"sms": b'{"new":true}'},
                    backup_directory=Path(directory),
                    lock_timeout=60,
                )

            redis_backups = list(Path(directory).glob("audience_spec_sms_*.json"))
            self.assertEqual(keys, ["yamata::audience_spec:cache:v4:sms"])
            self.assertEqual(len(redis_backups), 1)
            self.assertEqual(redis_backups[0].read_bytes(), b'{"old":true}')
            self.assertEqual(
                client.values["yamata::audience_spec:cache:v4:sms"], b'{"new":true}'
            )
            self.assertEqual(client.expiries["yamata::audience_spec:cache:v4:sms"], 300)
            lock = client.locks["yamata::audience_spec:rebuild-lock:v4:sms"]
            self.assertTrue(lock.acquired)
            self.assertTrue(lock.released)
            self.assertTrue(client.closed)


if __name__ == "__main__":
    unittest.main()
