FROM mongo:5
COPY build/mongo/mongo-import.sh /mongo-import.sh
COPY build/mongo/stock.json /stock.json
CMD /mongo-import.sh