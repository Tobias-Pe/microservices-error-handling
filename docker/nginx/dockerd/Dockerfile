FROM nginx:1.21.4-alpine
COPY config/nginx/dockerd.conf /nginx.conf.template
CMD ["/bin/sh" , "-c" , "envsubst < /nginx.conf.template > /etc/nginx/nginx.conf && exec nginx -g 'daemon off;'"]