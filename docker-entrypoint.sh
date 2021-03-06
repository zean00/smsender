#!/bin/bash

config=/smsender/config/config.yml
MYSQL_USER=${MYSQL_USER:-smsender_user}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-smsender_password}
MYSQL_HOST=${MYSQL_HOST:-db}
MYSQL_PORT=${MYSQL_PORT:-3306}
MYSQL_DATABASE=${MYSQL_DATABASE:-smsender}
NEXMO_KEY=${NEXMO_KEY:-key}
NEXMO_SECRET=${NEXMO_SECRET:-secret}
RAJASMS_KEY=${RAJASMS_KEY:-key}
RAJASMS_SERVER=${RAJASMS_SERVER:-localhost}
RAJASMS_MODE=${RAJASMS_MASKED:-otp}
RAJASMS_WEBHOOK_ENABLE=${RAJASMS_WEBHOOK_ENABLE:-false}
RAJASMS_WEBHOOK_SERVER=${RAJASMS_WEBHOOK_SERVER:-localhost}

echo "Configure config.yml"
if [ ! -f ${config} ]
then
  cp /config.default.yml ${config}
  sed -Ei "s/MYSQL_USER/$MYSQL_USER/" ${config}
  sed -Ei "s/MYSQL_PASSWORD/$MYSQL_PASSWORD/" ${config}
  sed -Ei "s/MYSQL_HOST/$MYSQL_HOST/" ${config}
  sed -Ei "s/MYSQL_PORT/$MYSQL_PORT/" ${config}
  sed -Ei "s/MYSQL_DATABASE/$MYSQL_DATABASE/" ${config}
  sed -Ei "s/NEXMO_KEY/$NEXMO_KEY/" ${config}
  sed -Ei "s/NEXMO_SECRET/$NEXMO_SECRET/" ${config}
  sed -Ei "s/RAJASMS_KEY/$RAJASMS_KEY/" ${config}
  sed -Ei "s/RAJASMS_SERVER/$RAJASMS_SERVER/" ${config}
  sed -Ei "s/RAJASMS_MODE/$RAJASMS_MODE/" ${config}
  sed -Ei "s/RAJASMS_WEBHOOK_ENABLE/$RAJASMS_WEBHOOK_ENABLE/" ${config}
  sed -Ei "s/RAJASMS_WEBHOOK_SERVER/$RAJASMS_WEBHOOK_SERVER/" ${config}
  echo OK
else
  echo SKIP
fi

echo "Wait until database $MYSQL_HOST:$MYSQL_PORT is ready..."
until nc -z ${MYSQL_HOST} ${MYSQL_PORT}
do
    sleep 1
done

sleep 1

echo "Starting app"
cd /smsender
./smsender -c ${config}