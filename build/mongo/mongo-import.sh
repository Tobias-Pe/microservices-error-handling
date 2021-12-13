#! /bin/bash

mongoimport --host mongodb --db mongo_db --collection stock --type json --file /stock.json --jsonArray