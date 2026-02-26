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
Test both deploy-local and deploy-beta.
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
yamata-local.conf is behind yamata-beta.conf

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

