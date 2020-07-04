# go-slack-bot-gateway
Gateway server for Slack apps.

## Install as service (CentOS)

```
sudo cp configs/slack-bot-gateway.service /etc/systemd/system/
sudo systemctl enable slack-bot-gateway
sudo systemctl start slack-bot-gateway
```