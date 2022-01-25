# Test Scenario

Basic Application with:
- Distribution managed by Docker-Swarm
- Timeouts 1 Second

## Result

CPU-Node:

- System resources not fully used --> still have capacity
- One node constantly at max (while load tests run)

Locust report: 

- Stock Service high occupancy: Many requests GET /articles/
- high failure rate in get articles

RequestsPart1: 

- Due to the high GET requests on /articles, the stock service is so busy handling the GET requests that it neglects the order-transactions
  - --> Orders accumulate massively in the status reserving
  - --> Hardly any utilization in shipment & payment & email

## Improvement Approach

There are two Problems:
1. The system resources need to be used more efficiently
2. The Stock Service needs a Bulkhead between Order and Stock Requests

Nr.1: More manually assign Containers to Nodes via "placement.constraints" --> See Testrun2

Nr.2: Introduce new Service as Stock Cache (Catalogue Service) --> See Testrun3