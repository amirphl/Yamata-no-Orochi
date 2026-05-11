install pg, redis, pgcli
psql "postgresql://mac:postgres@localhost:5432/somedb" -f migrations/run_all_up.sql
psql -U mac -d postgres -h localhost -p 5432
pgcli --host localhost --port 5432 --username mac --dbname somedb
htpasswd -bnBC 10 "" '378768kjhdfk7834JHDF' | tr -d ':\n'

insert into admins (uuid, username, password_hash) values ('7C4E8E07-6497-4F41-9D76-149CAC373280', 'admin', '$2y$10$kLEsquvG8Pf/TEcGFVx/HeD1kGitB1CFnOwZSl5YROqOVZ9btbvZC');

insert into bots (uuid, username, password_hash) values ('7C4E8E07-6497-4F41-9D76-149CAC373280', 'bot', '$2y$10$kLEsquvG8Pf/TEcGFVx/HeD1kGitB1CFnOwZSl5YROqOVZ9btbvZC');

update customers set is_mobile_verified = true where agency_referer_code = 'jazebeh.ir';

insert into line_numbers(name, line_number, price_factor, priority) values ('first line number', '2000023', 1.12, 1);

url -X POST "http://localhost:8080/api/v1/bot/auth/login" -H "Content-Type: application/json" -H "User-Agent: BotClient/1.0" -d '{"username": "bot", "password": "<password>"}'


-> Set url to localhost. Used for local audience spec 
set -x JAZEBEH_API_TOKEN <token>
python3 submit.py --platform sms
python3 submit.py --platform rubika
python3 submit.py --platform bale
python3 submit.py --platform splus




python3 jobs_by_category.py

python3 add_segment_price_factor.py --dbname somedb --user mac --password postgres

Add credit to your user in http://localhost:8081/satrap/sardis/payments

kernel sysctl

ssh keybased authentication, disable password based auth

enable firewall

psql "postgresql://mac:postgres@localhost:5432/somedb" -f migrations/run_all_up.sql