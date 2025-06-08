Jwt ttls must be read from config.
Exclude sensitive files from react docker app.
Test both deploy-local and deploy-beta.
------
template for sms
Upon 401 on protected apis, redirect to login page (remove local storage access tokens too).
Makefile
Create launch.json
How to apply new migrations? (dev, local, beta, etc)
Not all env vars are used in main app.
Fix tests.
SEO: robots.txt, favicon, etc
Tate limit is inconsistent in docker/nginx, scripts/ and fiber code.
    Also other configs of nginx like nopush.
Optimize docker/nginx configs.
Always display 401 do not expose extra info.

Install Git, Docker (add user to docker group), cron, acme.sh

# Option 1: Temporary fix
sudo sysctl vm.overcommit_memory=1

# Option 2: Permanent fix (add to /etc/sysctl.conf)
echo "vm.overcommit_memory = 1" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

10-listen-on-ipv6-by-default.sh: info: can not modify /etc/nginx/conf.d/default.conf (read-only file system?