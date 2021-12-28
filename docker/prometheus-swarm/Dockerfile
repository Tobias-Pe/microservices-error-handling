FROM prom/prometheus:v2.31.2

USER root

COPY config/prometheus-swarm/prometheus.yml /etc/prometheus/

COPY docker/prometheus-swarm/docker-entrypoint.sh /

RUN ["chmod", "+x", "/docker-entrypoint.sh"]

ENTRYPOINT [ "/docker-entrypoint.sh" ]
CMD        [ "--config.file=/etc/prometheus/prometheus.yml", \
             "--storage.tsdb.path=/prometheus" ]