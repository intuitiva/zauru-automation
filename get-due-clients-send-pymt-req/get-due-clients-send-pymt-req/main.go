package main

import (
	"bytes"
	"encoding/json" // marshal and unmarshal JSON
	"errors"        // errors
	"io/ioutil"     // Package ioutil implements some I/O utility functions (the response.Body is an io.ReadCloser...)
	"log"           // printf
	"net/http"      // GET POST
	"strconv"       // for string convertions
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Response is of type APIGatewayProxyResponse since we're leveraging the
// AWS Lambda Proxy Request functionality (default behavior)
//
// https://serverless.com/framework/docs/providers/aws/events/apigateway/#lambda-proxy-integration
type Response events.APIGatewayProxyResponse

// Clients definition hashes inside an array [{id: client_id, cat: client_category_id, seller: seller_id}, {...}]
type Client struct {
	Id     int64  `json:"id"`
	Info   string `json:"info"`
	Cat    int    `json:"cat"`
	Seller int    `json:"default_seller"`
	Due    string `json:"due"`
}

//Define a new structure that represents out API response (response status and body) nothing fancy but used in the channels
type HTTPResponse struct {
	Status string
	Body   string
	Client string
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
// It uses Amazon API Gateway request/responses provided by the aws-lambda-go/events package,
// However you could use other event sources (S3, Kinesis etc), or JSON-decoded primitive types such as 'string'.
func Handler(request events.APIGatewayProxyRequest) (Response, error) {

	// stdout and stderr are sent to AWS CloudWatch Logs
	log.Printf("Processing Lambda request %s\n", request.RequestContext.RequestID)

	if len(request.QueryStringParameters) < 1 {
		return Response{StatusCode: 404}, errors.New("no param were provided in the serverless function")
	}

	zauruUserEmail := ""
	zauruUserToken := ""
	excludeExclusiveSeller := 0
	excludeCat := 0
	// cycle thru params (for Zauru credentials, exclude exclusive seller, exclude payee_category)
	for k, v := range request.QueryStringParameters {
		if k == "ZauruUserEmail" {
			zauruUserEmail = v
		}
		if k == "ZauruUserToken" {
			zauruUserToken = v
		}
		if k == "ExcludeExclusiveSeller" {
			excludeExclusiveSeller, _ = strconv.Atoi(v)
		}
		if k == "ExcludeCat" {
			excludeCat, _ = strconv.Atoi(v)
		}
		log.Printf("GET param %s => %s\n", k, v)
	}

	if zauruUserEmail == "" || zauruUserToken == "" {
		return Response{StatusCode: 404}, errors.New("No Zauru credentials were provided ZauruUserToken or ZauruUserEmail")
	} else {

		// get the JSON with the clients with overdue payments
		// [
		//	{id: client_id1, cat: client_category_id1, default_seller: seller_id1, info: client_info1, due: due2},
		//	{id: client_id2, cat: client_category_id2, default_seller: seller_id2, info: client_info2, due: due2},
		//	...
		// ]
		clientsRequest, _ := http.NewRequest("GET", "https://app.zauru.com/sales/reports/clients_with_overdue_payments.json", nil)
		clientsRequest.Header.Add("Content-Type", "application/json")
		clientsRequest.Header.Add("X-User-Email", zauruUserEmail)
		clientsRequest.Header.Add("X-User-Token", zauruUserToken)

		httpClient := &http.Client{}
		clientsResponse, clientsErr := httpClient.Do(clientsRequest)
		if clientsErr != nil {
			return Response{StatusCode: 404}, clientsErr
		} else {

			defer clientsResponse.Body.Close()
			body, bodyErr := ioutil.ReadAll(clientsResponse.Body) // reading the io.ReadCloser object (returns []byte)
			if bodyErr != nil {
				return Response{StatusCode: 404}, errors.New("No body or weird body was responded from the clients_request")
			} else {

				// start parsing the array of hashes (Client struct)
				var clients []Client
				json.Unmarshal(body, &clients)

				// Define a new channel where my goroutines will communicate
				var HTTPResponses []HTTPResponse

				// traveling thru all clients to GET the URLs for each one (implementing conditions with IF)
				for _, c := range clients {
					if c.Seller != excludeExclusiveSeller || c.Cat != excludeCat {

						u := "https://app.zauru.com/settings/deliverable_reports/immediate_delivery_to_me.json?r_url=sales/reports/client_pending_payments&r_params[client]=" + strconv.FormatInt(c.Id, 10) + "&p_id=" + strconv.FormatInt(c.Id, 10) + "&r_name=ClientPendingPayments"
						log.Printf(u)
						httpClient2 := &http.Client{}
						// Execute the HTTP get
						reportRequest, _ := http.NewRequest("GET", u, nil)
						reportRequest.Header.Add("Content-Type", "application/json")
						reportRequest.Header.Add("X-User-Email", zauruUserEmail)
						reportRequest.Header.Add("X-User-Token", zauruUserToken)
						reportResponse, reportErr := httpClient2.Do(reportRequest)
						if reportErr != nil {
							log.Printf(reportErr.Error() + " " + u)
							HTTPResponses = append(HTTPResponses, HTTPResponse{"404", reportErr.Error(), c.Info})
						} else {
							defer reportResponse.Body.Close()
							reportBody, reportBodyErr := ioutil.ReadAll(reportResponse.Body)
							if reportBodyErr != nil {
								log.Printf(reportBodyErr.Error() + " " + u)
								HTTPResponses = append(HTTPResponses, HTTPResponse{"404", reportBodyErr.Error(), c.Info})
							} else {
								//Send an HTTPResponse back to the array of
								var reportBodyBuffer bytes.Buffer
								json.HTMLEscape(&reportBodyBuffer, reportBody)
								HTTPResponses = append(HTTPResponses, HTTPResponse{reportResponse.Status, strings.Join(strings.Split(reportBodyBuffer.String(), "\n"), ""), c.Info})
							}
						}
					}
				}

				//params := url.Values{}
				//params.Set("login", "a")
				//params.Set("password", "b")
				//postData := strings.NewReader(params.Encode())

				// prepare the JSON with all responses to respond to the request
				responsesJson, errResponsesJson := json.Marshal(HTTPResponses)
				if errResponsesJson != nil {
					log.Fatal("Cannot encode to JSON ", errResponsesJson)
				}
				var responsesBuffer bytes.Buffer
				json.HTMLEscape(&responsesBuffer, responsesJson)
				log.Printf("Enviados " + strconv.Itoa(len(HTTPResponses)) + " correos!!!")
				log.Printf(responsesBuffer.String())

				resp := Response{
					StatusCode:      200,
					IsBase64Encoded: false,
					Body:            responsesBuffer.String(),
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				}

				return resp, nil
			}
		}
	}
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
