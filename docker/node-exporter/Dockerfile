FROM prom/node-exporter:v1.3.1

ENV NODE_ID=none

USER root

COPY docker/node-exporter/docker-entrypoint.sh /etc/node-exporter/

ENTRYPOINT  [ "/etc/node-exporter/docker-entrypoint.sh" ]
CMD [ "/bin/node_exporter" ]