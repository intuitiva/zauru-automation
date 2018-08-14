# Get overdue clients and send payment request by email
A trigger function (`start` function) that fills out a Queue (SQS) that is consumed (by a `mail` function) and makes a GET request (serial not parallel as we dont want to kill Zauru) to Zauru that automatically sends the payment request by email.

We separated this function in 2 because AWS APIGatewayProxyRequest only allow 30 seconds to generate a response and if the response is issued, no more code can run in the function. We connected both functions via SQS to make the second function to last the 300 seconds that are available.

## start function
Gets the params via HTTP GET to schedule the necesary GETs to send to Zauru

> ### params
> * ZauruUserEmail - required (x@zauru.com)
> * ZauruUserToken - required (SKD9lskjdf2923e)
> * ExcludeExclusiveSeller - optional 
> * ExcludeCat - optional

## mail function

Gets the list of URLs to call from SQS (filled up by the other function `start`).

### Notices
 1 install dot_env node module to enable the env variables to be pushed to lambda with the serverless framework