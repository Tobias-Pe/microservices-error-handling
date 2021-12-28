FROM nginx:1.21.4-alpine
COPY config/nginx/cart.conf /nginx.conf.template
RUN ["/bin/sh" , "-c" , "envsubst < /nginx.conf.template > /etc/nginx/nginx.conf"]
CMD ["nginx", "-g", "daemon off;"]