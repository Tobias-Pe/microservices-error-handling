FROM mongo:5.0
COPY build/mongo/mongo-import.sh /mongo-import.sh
COPY build/mongo/stock.json /stock.json
CMD /mongo-import.sh