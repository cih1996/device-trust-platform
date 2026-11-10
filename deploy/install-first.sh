#!/bin/sh
set -eu
release=/opt/devicetrust/releases/0.4.1
test ! -e /opt/devicetrust
test ! -e /etc/nginx/sites-available/devicetrust
test ! -e /etc/nginx/sites-enabled/devicetrust
getent passwd devicetrust >/dev/null || useradd --system --home-dir /var/lib/devicetrust --shell /usr/sbin/nologin devicetrust
install -d -m 755 "$release"
tar -xzf /home/ubuntu/devicetrust-upload-20261111/release.tar.gz -C "$release"
chown -R root:root /opt/devicetrust
chmod 755 "$release"/bin/*
ln -s "$release" /opt/devicetrust/current
install -d -m 700 /opt/devicetrust/backups
tar -czf /opt/devicetrust/backups/nginx-before-0.4.1.tar.gz -C /etc/nginx nginx.conf sites-available sites-enabled conf.d
for domain in devicetrust.lingzidev.com devicetrust-console.lingzidev.com devicetrust-sdk-api.lingzidev.com; do
    install -d -m 700 "/etc/nginx/ssl/devicetrust/$domain"
    install -m 644 "/home/ubuntu/devicetrust-upload-20261111/$domain.pem" "/etc/nginx/ssl/devicetrust/$domain/fullchain.pem"
    install -m 600 "/home/ubuntu/devicetrust-upload-20261111/$domain.key" "/etc/nginx/ssl/devicetrust/$domain/privkey.pem"
done
install -d -m 700 -o devicetrust -g devicetrust /var/lib/devicetrust
runuser -u devicetrust -- "$release/bin/setup-demo" -data /var/lib/devicetrust -ca /etc/ssl/certs/ca-certificates.crt -build-info "$release/build-info.json"
python3 - <<'PY'
import json,os
p='/var/lib/devicetrust/demo.json'
data=json.load(open(p))
data['platformUrl']='https://devicetrust-sdk-api.lingzidev.com'
with open(p,'w') as f:json.dump(data,f,indent=2)
os.chmod(p,0o600)
PY
runuser -u devicetrust -- "$release/bin/trust-sync" -out /var/lib/devicetrust/trust
install -m 644 "$release"/deploy/*.service "$release"/deploy/*.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now devicetrust-platform devicetrust-demo devicetrust-trust.timer
curl --fail --silent http://127.0.0.1:18191/health
curl --fail --silent http://127.0.0.1:18192/health
install -m 644 "$release/deploy/devicetrust.nginx" /etc/nginx/sites-available/devicetrust
ln -s /etc/nginx/sites-available/devicetrust /etc/nginx/sites-enabled/devicetrust
nginx -t
systemctl reload nginx
systemctl is-active devicetrust-platform devicetrust-demo
