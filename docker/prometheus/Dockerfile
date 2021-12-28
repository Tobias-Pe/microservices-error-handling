FROM prom/prometheus:v2.31.2

COPY config/prometheus.yml /etc/prometheus/
COPY docker/prometheus/docker-entrypoint.sh /etc/prometheus/

ENTRYPOINT [ "/etc/prometheus/docker-entrypoint.sh" ]
CMD        [ "--config.file=/etc/prometheus/prometheus.yml", \
             "--storage.tsdb.path=/prometheus" ]