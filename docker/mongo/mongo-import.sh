#!/bin/bash

mongoimport --uri "mongodb://stock-mongodb/mongo_db?replicaSet=stockDB0" --db mongo_db --collection stock --type json --file /stock.json --jsonArray