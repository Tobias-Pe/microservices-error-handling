#!/bin/bash

mongoimport --host stock-mongodb --db mongo_db --collection stock --type json --file /stock.json --jsonArray