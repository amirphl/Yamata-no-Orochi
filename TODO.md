Run gentime.py which generates something like this:
025-12-19 09:01:28
2025-12-19 14:21:51
2025-12-19 15:50:00
2025-12-19 17:27:59
2025-12-19 18:56:50
....
Then run git diff --cached for recommending git commit message per file separately in logiccal order.
Output each commit message in following format so that i can easily copy paste:
git commit --date="<date from output of that file>" <file_path> --m "<commit message>"

-------------

Jwt ttls must be read from config.
Exclude sensitive files from react docker app.
Test deploy-beta.
Upon 401 on protected apis, redirect to login page (remove local storage access tokens too).
Makefile
Not all env vars are used in main app.
SEO: robots.txt, favicon, etc
Tate limit is inconsistent in docker/nginx, scripts/ and fiber code.
    Also other configs of nginx like nopush.
Optimize docker/nginx configs.
Install Git, Docker (add user to docker group), cron, acme.sh

# Option 1: Temporary fix
sudo sysctl vm.overcommit_memory=1

# Option 2: Permanent fix (add to /etc/sysctl.conf)
echo "vm.overcommit_memory = 1" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

10-listen-on-ipv6-by-default.sh: info: can not modify /etc/nginx/conf.d/default.conf (read-only file system?

------------

provider.GetDeposits might return duplicate messages
auto verify of crypto payments
env vars in docker .env
check user deposited enough coins
what happens in case of multiple deposit
test oxapay api calls
// Not using polling for now; Oxapay supports callbacks and history endpoints
func (c *OxapayClient) GetDeposits(ctx context.Context, providerRequestID string) ([]DepositInfo, error) {
	return nil, errors.New("oxapay: GetDeposits not implemented; use webhook or history endpoint mapping")
}

func (c *OxapayClient) VerifyTx(ctx context.Context, txHash string) (*DepositInfo, error) {
	return nil, errors.New("oxapay: VerifyTx not implemented; use webhook or history endpoint mapping")
}

quote by TMN, callback from oxapay
double spend problem at f.creditOnConfirmed


segment and subsegment to level 1 level 2 level 3 inside (double check callers)
  GetCampaignResponse
  AdminGetCampaignResponse
  BotGetCampaignResponse
proper error handling in campaign_handler.go


Webhook integration, lock on sequential uids, default schema for campaigns features, What if available audience is less than user selected num audience? What if sent less than available audience?
QA sheets (local, drive), setup Alireza system for front end development
summary API called twice, 401 loop on api (fetch phone numbers), display click rate to user, fix Alireza apis,
Fix generated excel missing sheet


fix jaz sms reporting
refactor front end

https://app.oxapay.com/merchant-service
https://docs.oxapay.com/webhook
- Admin permission hardening (open items)
  - Add MFA to admin login flow (TOTP/SMS/Email OTP) after captcha/password; store per-admin MFA secret/status; enforce on login.
  - Add step-up MFA for high-risk actions (payment:*, platform-base-price:create, acl:approve, audit exports) requiring fresh MFA within N minutes.
  - Shorten admin access-token TTL; rotate refresh-token secrets regularly and revoke on ACL change or password change.
  - Require dual approval for superadmin grants or restrict acl:approve for superadmin role changes to designated approvers.
  - Provide UI/CRUD to assign roles/overrides plus read-only audit trail of ACL changes (API exists; UI missing).
  - Implement step-up guard details: define HighRiskPermissions set (payment:*, platform-base-price:create, acl:approve, audit export); extend AdminAuthorize to enforce recent-MFA flag (else 403 MFA_REQUIRED); persist last_admin_mfa_at with short TTL (10–15 min); ensure login MFA flow sets the flag and re-prompts when expired.
- Auditing & observability gaps
  - Emit audit events for permission checks (success/failure with admin_id, permission key, target route/resource).
  - Add monitoring/dashboard/alerts for permission-deny spikes to catch misconfigurations.
- Testing & safety rails
  - Add unit tests that walk RoutePermissionRegistry vs router to ensure all admin routes are mapped and guarded.
  - Dry-run mode in lower envs to log routes missing permission mappings when new endpoints are added.
  - Periodic “effective ACL” report per admin to detect privilege creep (roles + allows/denies + derived permissions).
- Migration path
  - Start with role-based enforcement; then add per-admin allow/deny overrides (keep report-only toggle to ease rollout).
  - Backfill permission map for existing routes and run in “report-only” mode to surface gaps before strict enforcement.
