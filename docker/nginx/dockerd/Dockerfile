FROM nginx:1.21.4-alpine
COPY config/nginx/dockerd.conf /nginx.conf.template
EXPOSE 9323
CMD ["/bin/sh" , "-c" , "envsubst < /nginx.conf.template > /etc/nginx/nginx.conf && exec nginx -g 'daemon off;'"]