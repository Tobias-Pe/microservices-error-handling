FROM mongo:5
COPY docker/mongo/mongo-import.sh /mongo-import.sh
RUN chmod +x /mongo-import.sh
COPY docker/mongo/stock.json /stock.json
CMD /mongo-import.sh