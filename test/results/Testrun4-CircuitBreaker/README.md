# Test Scenario

- 4 Circuit breaker (CB) inside API gateway
  - Order CB
  - Currency CB
  - Cart CB
  - Catalogue CB
- CB Failure Rate threshold: 25%
- CB switch to half open state timeout: 1.5 Seconds
- 2.5 Second general timeout
- Improvements of the previous Testruns

## Result



CPU-Node:

- CPU utilization per node is protected by the circuit breaker at high loads

CPU-Service:

- API-Gateway Utilization increased

CircuitBreakerAPI-Gateway:

_Disclaimer for this metric: Circuit Breaker open states which are shorter than the prometheus scrape interval can be missed out!_

- Long open states

RequestsTotal1:

- Collapses of Requests through opening of CB's inside API-Gateway

Locust-Report:

- Increased Error-Rate
- Increased RPS

Now, nodes will no longer be at almost 100% utilization for a longer period of time, but by tripping the circuit breakers,
the node is protected from this overload and has time to recover.

The increase in RPS could be influenced by the fast return of an error from the api-gateway.

## Improvements

Nevertheless, there are long periods in which all circuit breakers are in the Open state. 
This could be due to incorrect configuration of the timeouts.

If the timeout is set too long, 
it would result in a large amount of coroutines wanting to work in the client. 
The client can process a limited number of requests with maximum exhaustion of the given resources. 
All unprocessed requests within the timeout period are aborted and the server waited the timeout period 
in vain. The waiting time would hardly be a problem for the server, since the wait was within the coroutine,
but if the request made by the server is customer-relevant and the customer also wants a response, 
a long waiting time is bad. 

Furthermore, timeouts that are too long or too short can affect the error rate of a request. 
In combination with circuit breakers, this can lead to incorrect assumptions and unnecessarily 
increase the error rate due to an open circuit breaker condition.

An improvement could be to use adaptive timeouts --> See Testrun5

