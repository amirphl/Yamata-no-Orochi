import argparse
import contextlib
import io
import unittest
from unittest import mock

import recover_sms_campaign_statistics as recovery


class RecoverSMSCampaignStatisticsTests(unittest.TestCase):
    def test_campaign_scope_is_fixed(self):
        self.assertEqual(
            recovery.CAMPAIGN_IDS,
            (935, 944, 945, 943, 937, 933, 942),
        )
        with contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit):
                recovery.parse_args(
                    [
                        "--campaign-ids",
                        "1",
                        "--db-name",
                        "db",
                        "--db-user",
                        "user",
                    ]
                )

    def test_aggregate_matches_scheduler_sql_shape(self):
        statuses = [
            {
                "total_parts": 2,
                "total_delivered_parts": 2,
                "total_undelivered_parts": 0,
                "total_unknown_parts": 0,
            },
            {
                "total_parts": 2,
                "total_delivered_parts": 1,
                "total_undelivered_parts": 1,
                "total_unknown_parts": 0,
            },
            {
                "total_parts": 0,
                "total_delivered_parts": 0,
                "total_undelivered_parts": 0,
                "total_unknown_parts": 0,
            },
            {
                "total_parts": None,
                "total_delivered_parts": None,
                "total_undelivered_parts": None,
                "total_unknown_parts": None,
            },
        ]
        stats = recovery.aggregate_statuses(statuses)
        self.assertEqual(stats["aggregatedTotalRecords"], 4)
        self.assertEqual(stats["aggregatedTotalSent"], 2)
        self.assertEqual(stats["aggregatedTotalParts"], 4)
        self.assertEqual(stats["aggregatedTotalDeliveredParts"], 3)
        self.assertEqual(stats["aggregatedTotalUnDeliveredParts"], 1)
        self.assertEqual(stats["aggregatedTotalUnKnownParts"], 0)

    def test_provider_cannot_inject_an_unrequested_tracking_id(self):
        parsed, ignored = recovery.parse_status_items(
            [
                {
                    "customerId": "expected",
                    "serverId": "server-1",
                    "totalParts": 1,
                    "totalDeliveredParts": 1,
                    "totalUnDeliveredParts": 0,
                    "totalUnKnownParts": 0,
                    "status": "Delivered",
                },
                {
                    "customerId": "other-campaign",
                    "totalParts": 999,
                    "totalDeliveredParts": 999,
                    "totalUnDeliveredParts": 0,
                    "totalUnKnownParts": 0,
                    "status": "Delivered",
                },
            ],
            {"expected"},
        )
        self.assertEqual(set(parsed), {"expected"})
        self.assertEqual(ignored, 1)

    def test_confirmation_is_required_for_noninteractive_push(self):
        args = argparse.Namespace(push=True, yes=False)
        with mock.patch.object(recovery.sys.stdin, "isatty", return_value=False):
            with self.assertRaisesRegex(RuntimeError, "requires --yes"):
                recovery.confirm_push(args)

    def test_payam_password_is_posted_as_form_data_not_url(self):
        response = mock.MagicMock(status_code=200)
        response.json.return_value = {"access_token": "token"}
        session = mock.MagicMock()
        session.post.return_value = response
        config = {
            "token_url": "https://sms.example/token",
            "status_url": "https://sms.example/status",
            "system_name": "system",
            "username": "user",
            "password": "secret-password",
            "scope": "webservice",
            "grant_type": "password",
            "root_access_token": "",
        }
        self.assertEqual(recovery.payam_login(session, config, 5), "token")
        call = session.post.call_args
        self.assertEqual(call.args[0], "https://sms.example/token")
        self.assertEqual(call.kwargs["data"]["password"], "secret-password")
        self.assertNotIn("secret-password", call.args[0])


if __name__ == "__main__":
    unittest.main()
