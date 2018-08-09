# Zauru Automation via serverless technology

Golang server less functions in AWS lamda for Zauru to automate various tasks and communicate with other services (including our own)

The reason for this repo is to show all the integrations we can do, in our ERP, our mailer app, our CRM and how we communicate with other services.

This functions can be called from Zapier to have a log and make it easy to handle.

Always clone this repo and insert the repository whole into a new folder and rename the parent folder from cirio-automator to src to comply with https://golang.org/doc/code.html

That implies that you must set your GOPATH env variable with ```export PATH=$PATH:(parent of the src folder)```

Conditions to make serverless functions
* Always use Zapier as the gateway to register each function (scheduled or webhook endpoint) that way we will have an accessible LOG.
* No user email or user token keys are hardcoded, everything must come as a PARAM to the function, for reusability and privacy
* SQS credentials are stored in the .env
