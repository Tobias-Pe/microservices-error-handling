FROM prom/node-exporter:v1.3.1

ENV NODE_ID=none

USER root

COPY docker/node-exporter/docker-entrypoint.sh /etc/node-exporter/
RUN ["chmod", "+x", "/etc/node-exporter/docker-entrypoint.sh"]

ENTRYPOINT  [ "/etc/node-exporter/docker-entrypoint.sh" ]
CMD [ "/bin/node_exporter" ]