# Test Scenario

- Catalog Bulkhead added
- Improvements of Testrun2

## Results

CPU-Services:

- Stock Service not anymore most CPU-Using Service

Locust-Report:

- Error rate has dropped but so has the RPS

Through the new Catalogue Service, which is distributed globally on all nodes of the testing setup, the setup resources utilization has increased.
Therefore, the RPS dropped.

Now if the Catalogue Service is flooded with requests, it will not slow down the Order-Transaction flow.
