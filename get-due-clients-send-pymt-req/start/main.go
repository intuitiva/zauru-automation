package main

import (
	"encoding/json" // marshal and unmarshal JSON
	"errors"        // errors
	"io/ioutil"     // Package ioutil implements some I/O utility functions (the response.Body is an io.ReadCloser...)
	"log"           // printf
	"net/http"      // GET POST
	"os"            // getting env variables
	"strconv"       // for string convertions
	"strings"       // simple functions to manipulate UTF-8 encoded strings

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
	Id       int64  `json:"id"`
	Info     string `json:"info"`
	Cat      string `json:"cat"`
	Seller   string `json:"default_seller"`
	Due      string `json:"due"`
	Currency string `json:"currency"`
}

// list of urls + POST params, some stuff will repeat (user_email, user_token, method) in all requests
type ListOfUrls struct {
	Method         string   `json:"method"`
	ZauruUserEmail string   `json:"zauru_user_email"`
	ZauruUserToken string   `json:"zauru_user_token"`
	Urls           []string `json:"urls"`
	Body           []string `json:"body"` // this will contain the JSON with email subject, body, report params, etc.
}

// JSON for the POST params to send
type Rparams struct {
	Client string `json:"client"`
}

type Params struct {
	Pid     string  `json:"p_id"`
	Rbody   string  `json:"r_body"`
	Rname   string  `json:"r_name"`
	Rurl    string  `json:"r_url"`
	Rparams Rparams `json:"r_params"`
}

func intNotInSlice(i int, list []int) bool {
	for _, v := range list {
		// short circuit evaluation
		if v == i {
			return false
		}
	}
	return true
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
	emailSubject := ""
	emailBody := ""
	var excludeExclusiveSeller = []int{}
	var excludeCat = []int{}
	// cycle thru params (for Zauru credentials, exclude exclusive seller, exclude payee_category)
	for k, v := range request.QueryStringParameters {
		if k == "ZauruUserEmail" {
			zauruUserEmail = v
		}
		if k == "ZauruUserToken" {
			zauruUserToken = v
		}
		if k == "ExcludeExclusiveSeller" {
			temp := strings.Split(v, "-")
			for _, i := range temp {
				j, err := strconv.Atoi(i)
				if err != nil {
					log.Printf(err.Error())
				} else {
					excludeExclusiveSeller = append(excludeExclusiveSeller, j)
				}
			}
		}
		if k == "ExcludeCat" {
			temp := strings.Split(v, "-")
			for _, i := range temp {
				j, err := strconv.Atoi(i)
				if err != nil {
					log.Printf(err.Error())
				} else {
					excludeCat = append(excludeCat, j)
				}
			}
		}
		if k == "EmailSubject" {
			emailSubject = v
		}
		if k == "EmailBody" {
			emailBody = v
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
						Method:         "POST",
						ZauruUserEmail: zauruUserEmail,
						ZauruUserToken: zauruUserToken,
					},
				}

				// traveling thru all clients to GET the URLs for each one (implementing conditions with IF)
				// sending batches of 20 URLS
				counter := 0
				u := "https://app.zauru.com/settings/deliverable_reports/immediate_delivery_to_payee.json"
				for _, c := range clients {
					////
					// CONDITIONS
					////
					seller, _ := strconv.Atoi(c.Seller)
					cat, _ := strconv.Atoi(c.Cat)
					if intNotInSlice(seller, excludeExclusiveSeller) && intNotInSlice(cat, excludeCat) && c.Currency == "GTQ" {

						prms := Params{
							Pid:   strconv.FormatInt(c.Id, 10),
							Rname: emailSubject,
							Rbody: emailBody,
							Rurl:  "sales/reports/client_pending_payments",
							Rparams: Rparams{
								Client: strconv.FormatInt(c.Id, 10),
							},
						}
						jsonParams, _ := json.Marshal(prms)
						log.Printf(string(jsonParams))
						index := (counter / 20) // starting from 0
						// grow listOfUrls slice
						if index >= len(listOfUrls) {
							listOfUrls = append(listOfUrls, ListOfUrls{
								Method:         "POST",
								ZauruUserEmail: zauruUserEmail,
								ZauruUserToken: zauruUserToken,
							})
						}
						listOfUrls[index].Urls = append(listOfUrls[index].Urls, u)
						listOfUrls[index].Body = append(listOfUrls[index].Body, string(jsonParams))
						counter++
					}
				}

				if len(listOfUrls) <= 0 {
					log.Printf("No body or weird body was responded from the clients_request")
					return Response{StatusCode: 500}, errors.New("No body or weird body was responded from the clients_request")
				} else {

					// Configuring SQS
					// Initialize a session that the SDK will use
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

func main() {
	lambda.Start(Handler)
}
