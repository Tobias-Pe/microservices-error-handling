# Test Scenario

- Adaptive Timeouts for all Request-Types inside API-Gateway
- Initial Timeout 1 Second
- Moving Average Windows Size 50
- tolerance threshold 1.5
- Improvements of the previous Testruns

# Results

CPU-Service:

- API-Gateway Utilization decreased

CircuitBreakerAPI-Gateway:

- No more long open states
- CB's better supported by adaptive Timeouts

RequestsTotal1:

- No more strong collapse of Requests through opening of CB's inside API-Gateway

Locust-Report:

- Decreased Error-Rate
- approximately same RPS

Timeouts:

- Fast adaption and recovering
- Keeps timeout low aslong system runs smoothly

The timeouts won't replace the self-healing effects of the CB's. They support them.
