# cirio-automator

Golang server less functions in AWS lamda for Zauru to automate various tasks and communicate with other services (including our own)

Always clone this repo and insert the repository whole into a new folder and rename the parent folder from cirio-automator to src to comply with https://golang.org/doc/code.html

That implies that you must set your GOPATH env variable with ```export PATH=$PATH:(parent of the src folder)```

Conditions to make serverless functions
* Always use Zapier as the gateway to register each function (scheduled or webhook endpoint) that way we will have an accessible LOG.
* No user email or user token keys are hardcoded, everything must come as a PARAM to the function, for reusability and privacy
* SQS credentials are stored in the .env
