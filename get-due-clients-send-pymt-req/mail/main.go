package main

import (
	"bytes"
	"encoding/json" // marshal and unmarshal JSON
	"io/ioutil"     // Package ioutil implements some I/O utility functions (the response.Body is an io.ReadCloser...)
	"log"           // printf
	"net/http"      // GET POST
	"strconv"       // for string convertions
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Clients definition hashes inside an array [{id: client_id, cat: client_category_id, seller: seller_id}, {...}]
type ListOfUrls struct {
	Method         string   `json:"method"`
	ZauruUserEmail string   `json:"zauru_user_email"`
	ZauruUserToken string   `json:"zauru_user_token"`
	Message        string   `json:"message"`
	Urls           []string `json:"urls"`
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
		for _, c := range listOfUrls.Urls {
			httpClient2 := &http.Client{}
			// Execute the HTTP get
			reportRequest, _ := http.NewRequest(listOfUrls.Method, c, nil)
			reportRequest.Header.Add("Content-Type", "application/json")
			reportRequest.Header.Add("X-User-Email", zauruUserEmail)
			reportRequest.Header.Add("X-User-Token", zauruUserToken)
			reportResponse, reportErr := httpClient2.Do(reportRequest)
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

//func DoHTTPPost(url string, param map[string]string, ch chan<- HTTPResponse) {
//	jsonValue, _ := json.Marshal(param)
//	httpClient2 := &http.Client{}
//	reportRequest, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonValue))
//	reportRequest.Header.Add("Content-Type", "application/json")
//	reportRequest.Header.Add("X-User-Email", zauruUserEmail)
//	reportRequest.Header.Add("X-User-Token", zauruUserToken)
//	reportResponse, _ := httpClient2.Do(reportRequest) // avoid catching errors
//	reportBody, _ := ioutil.ReadAll(reportResponse.Body)
//	ch <- HTTPResponse{reportResponse.Status, reportBody}
//}

func main() {
	lambda.Start(Handler)
}
