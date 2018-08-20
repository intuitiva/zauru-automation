package main

import (
	"bytes"         // functions for the manipulation of byte slices
	"encoding/json" // marshal and unmarshal JSON
	"io/ioutil"     // Package ioutil implements some I/O utility functions (the response.Body is an io.ReadCloser...)
	"log"           // printf
	"net/http"      // GET POST
	"strconv"       // for string convertions
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// list of urls + POST params, some stuff will repeat (user_email, user_token, method) in all requests
type ListOfUrls struct {
	Method         string   `json:"method"`
	ZauruUserEmail string   `json:"zauru_user_email"`
	ZauruUserToken string   `json:"zauru_user_token"`
	Urls           []string `json:"urls"`
	Body           []string `json:"body"` // this will contain the JSON with email subject, body, report params, etc.
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
// It uses Amazon SQS request/responses provided by the aws-lambda-go/events package,
// However you could use other event sources (S3, Kinesis etc), or JSON-decoded primitive types such as 'string'.
func Handler(sqsEvent events.SQSEvent) (string, error) {

	message := sqsEvent.Records[0] // just 1 record as my batch size is 1 (serverless.yml)

	var listOfUrls ListOfUrls
	json.Unmarshal([]byte(message.Body), &listOfUrls)
	// for Zauru credentials, exclude exclusive seller, exclude payee_category
	zauruUserEmail := listOfUrls.ZauruUserEmail
	zauruUserToken := listOfUrls.ZauruUserToken

	if zauruUserEmail == "" || zauruUserToken == "" {
		log.Printf("No Zauru credentials were provided ZauruUserToken or ZauruUserEmail")
		return "No Zauru credentials were provided ZauruUserToken or ZauruUserEmail", nil
	} else {

		// traveling thru all clients to GET the URLs for each one (implementing conditions with IF)
		for i, c := range listOfUrls.Urls {
			httpClient := &http.Client{}
			// Execute the HTTP get
			reportRequest, _ := http.NewRequest(listOfUrls.Method, c, bytes.NewBufferString(listOfUrls.Body[i]))
			reportRequest.Header.Add("Content-Type", "application/json")
			reportRequest.Header.Add("X-User-Email", zauruUserEmail)
			reportRequest.Header.Add("X-User-Token", zauruUserToken)
			reportResponse, reportErr := httpClient.Do(reportRequest)
			if reportErr != nil {
				log.Printf(reportErr.Error() + " " + c)
				//return reportErr.Error() + " " + c, reportErr
			} else {
				defer reportResponse.Body.Close()
				reportBody, reportBodyErr := ioutil.ReadAll(reportResponse.Body)
				if reportBodyErr != nil {
					log.Printf(reportBodyErr.Error() + " " + c)
					//return reportBodyErr.Error() + " " + c, reportBodyErr
				} else {
					////
					// ON SUCCESS, (passing all validations) just print the response
					////
					var reportBodyBuffer bytes.Buffer
					json.HTMLEscape(&reportBodyBuffer, reportBody)
					log.Printf("%s -> %s", reportResponse.Status, strings.Join(strings.Split(reportBodyBuffer.String(), "\n"), ""), c)
				}
			}
		}
		log.Printf("Enviados " + strconv.Itoa(len(listOfUrls.Urls)) + " correos!!!")
	}

	return "Hoy si terminamos", nil
}

func main() {
	lambda.Start(Handler)
}
