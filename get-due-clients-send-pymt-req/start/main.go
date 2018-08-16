package main

import (
	"encoding/json" // marshal and unmarshal JSON
	"errors"        // errors
	"io/ioutil"     // Package ioutil implements some I/O utility functions (the response.Body is an io.ReadCloser...)
	"log"           // printf
	"net/http"      // GET POST
	"os"            // getting env variables
	"strconv"       // for string convertions

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

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

type ListOfUrls struct {
	Method         string   `json:"method"`
	ZauruUserEmail string   `json:"zauru_user_email"`
	ZauruUserToken string   `json:"zauru_user_token"`
	Message        string   `json:"message"`
	Urls           []string `json:"urls"`
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

				// Define a new slice of objects that will be pushed to SQS
				// initialize first element of slice
				var listOfUrls = []ListOfUrls{
					ListOfUrls{
						Method:         "GET",
						ZauruUserEmail: zauruUserEmail,
						ZauruUserToken: zauruUserToken,
						Message:        "Hola",
					},
				}

				// traveling thru all clients to GET the URLs for each one (implementing conditions with IF)
				// sending batches of 20 URLS
				counter := 0
				for _, c := range clients {
					if c.Seller != excludeExclusiveSeller && c.Cat != excludeCat {

						u := "https://app.zauru.com/settings/deliverable_reports/immediate_delivery_to_me.json?r_url=sales/reports/client_pending_payments&r_params[client]=" + strconv.FormatInt(c.Id, 10) + "&p_id=" + strconv.FormatInt(c.Id, 10) + "&r_name=ClientPendingPayments"
						//log.Printf(u)
						index := (counter / 20) // starting from 0
						// grow listOfUrls slice
						if index >= len(listOfUrls) {
							listOfUrls = append(listOfUrls, ListOfUrls{Method: "GET", ZauruUserEmail: zauruUserEmail, ZauruUserToken: zauruUserToken, Message: "Hola"})
						}
						listOfUrls[index].Urls = append(listOfUrls[index].Urls, u)
						counter++
					}
				}

				if len(listOfUrls) <= 0 {
					log.Printf("No body or weird body was responded from the clients_request")
					return Response{StatusCode: 500}, errors.New("No body or weird body was responded from the clients_request")
				} else {

					// Configuring SQS
					// Initialize a session that the SDK will use to load credentials from the shared credentials file, ~/.aws/credentials.
					svc := sqs.New(session.New(), &aws.Config{Region: aws.String("us-west-2")})

					// URL to our queue
					qURL := os.Getenv("URL_QUEUE_AUTOMATION_GET_DUE_CLIENTS_SEND_PYMENT_REQ")

					// Sending SQS messages with the body as the ListOfUrl in JSON format
					for _, lou := range listOfUrls {
						jsn, errJson := json.Marshal(lou)
						if errJson != nil {
							log.Printf(errJson.Error())
							return Response{StatusCode: 500}, errJson
						} else {
							result, errSQSSend := svc.SendMessage(&sqs.SendMessageInput{
								DelaySeconds: aws.Int64(10),
								MessageBody:  aws.String(string(jsn)),
								QueueUrl:     &qURL,
							})
							if errSQSSend != nil {
								log.Printf(errSQSSend.Error())
								return Response{StatusCode: 500}, errSQSSend
							} else {
								log.Printf(*result.MessageId)
							}
						}
					}

					resultado := "Se enviaran " + strconv.Itoa(len(listOfUrls)) + " paquetes de requests con un total de " + strconv.Itoa(counter) + " requests !!!"
					log.Printf(resultado)

					resp := Response{
						StatusCode:      200,
						IsBase64Encoded: false,
						Body:            resultado,
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
					}

					return resp, nil
				}
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
