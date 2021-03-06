#!/bin/sh -e

cat /etc/prometheus/prometheus.yml > /tmp/prometheus.yml

if [ ${JOBS+x} ]; then

for job in $JOBS
do
echo "adding job $job"

SERVICE=$(echo "$job" | cut -d":" -f1)
PORT=$(echo "$job" | cut -d":" -f2)

cat >>/tmp/prometheus.yml <<EOF


  - job_name: '${SERVICE}'
    dns_sd_configs:
    - names:
      - 'tasks.${SERVICE}'
      type: 'A'
      port: ${PORT}
EOF

done

fi

mv /tmp/prometheus.yml /etc/prometheus/prometheus.yml

set -- /bin/prometheus "$@"

exec "$@"