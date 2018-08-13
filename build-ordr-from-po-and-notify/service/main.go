package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
)

type RequestParams struct {
	Purchase_order_id    int
	Payment_term_id      int
	Seller_id            int
	Agency_id            int
	Recipient_email      string
	Sender_email         string
	Email_title          string
	Email_entity_logo    string
	Email_entity_name    string
	Email_recipient_name string
	Email_extra_cc       string
	Email_extra_bcc      string
}

type poData struct {
	Reference                  string                            `json:"reference"`
	Memo                       string                            `json:"memo"`
	Date                       string                            `json:"date"`
	Taxable                    bool                              `json:"taxable"`
	Pos                        bool                              `json:"pos"`
	Payee_id                   string                            `json:"payee_id"`
	Order_number               string                            `json:"order_number"`
	Payment_term_id            int                               `json:"payment_term_id"`
	Seller_id                  int                               `json:"seller_id"`
	Agency_id                  int                               `json:"agency_id"`
	Invoice_details_attributes map[string]map[string]interface{} `json:"invoice_details_attributes"`
}

type poStruct struct {
	Invoice poData `json:"invoice"`
}

// This function make validations and return body params
func getParams(request *events.APIGatewayProxyRequest) (*RequestParams, error) {
	if request.Headers["X-User-Email-1"] == "" {
		return nil, errors.New(`user email (1) is missing.`)
	}

	if request.Headers["X-User-Token-1"] == "" {
		return nil, errors.New(`user token (1) is missing.`)
	}

	if request.Headers["X-User-Email-2"] == "" {
		return nil, errors.New(`user email (2) is missing.`)
	}

	if request.Headers["X-User-Token-2"] == "" {
		return nil, errors.New(`user token (2) is missing.`)
	}

	var params RequestParams

	if err := json.Unmarshal([]byte(request.Body), &params); err != nil {
		return nil, errors.New(err.Error())
	}

	if params.Purchase_order_id == 0 {
		return nil, errors.New(`purchase order id is missing.`)
	}

	if params.Payment_term_id == 0 {
		return nil, errors.New(`payment term id is missing.`)
	}

	if params.Seller_id == 0 {
		return nil, errors.New(`seller id is missing.`)
	}

	if params.Agency_id == 0 {
		return nil, errors.New(`agency id is missing.`)
	}

	if params.Recipient_email == "" {
		return nil, errors.New(`recipient email is missing.`)
	}

	if params.Email_title == "" {
		return nil, errors.New(`email title is missing.`)
	}

	if params.Email_recipient_name == "" {
		return nil, errors.New(`email recipient name is missing.`)
	}

	return &params, nil
}

func Handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Validate if api key and user email is not empty
	params, err := getParams(&request)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"getParams. %s"}`, err.Error()), StatusCode: 500}, nil
	}

	// Request configuration
	url_po := fmt.Sprintf("https://zauru.herokuapp.com/purchases/purchase_orders/%d.json", params.Purchase_order_id)

	req, err := http.NewRequest("GET", url_po, bytes.NewBuffer([]byte("")))
	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"http.NewRequest.po. %s"}`, err.Error()), StatusCode: 500}, nil
	}
	req.Header.Set("X-User-Email", request.Headers["X-User-Email-1"])
	req.Header.Set("X-User-Token", request.Headers["X-User-Token-1"])

	// Sending request to get purchase order data
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"client.Do.po. %s"}`, err.Error()), StatusCode: 500}, nil
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"ioutil.ReadAll.po. %s"}`, err.Error()), StatusCode: 500}, nil
	}

	// Parsing json PO response to object
	var po_object interface{}

	if err := json.Unmarshal(body, &po_object); err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"Invalid purchase order json response. %s"}`, err.Error()), StatusCode: 500}, nil
	}

	purchase_order := po_object.(map[string]interface{})

	// Filling sale order data
	so_data := &poData{
		Reference:                  purchase_order["reference"].(string),
		Memo:                       purchase_order["memo"].(string),
		Date:                       purchase_order["issue_date"].(string),
		Taxable:                    true,
		Pos:                        false,
		Payee_id:                   purchase_order["agency"].(map[string]interface{})["notes"].(string),
		Order_number:               fmt.Sprintf("%f", purchase_order["zid"].(float64)),
		Payment_term_id:            params.Payment_term_id,
		Seller_id:                  params.Seller_id,
		Agency_id:                  params.Agency_id,
		Invoice_details_attributes: make(map[string]map[string]interface{}),
	}
	so_object := &poStruct{
		Invoice: *so_data,
	}

	row_table := ""
	for i, po := range purchase_order["purchase_order_details"].([]interface{}) {
		so_object.Invoice.Invoice_details_attributes[fmt.Sprintf("%d", i)] = make(map[string]interface{})
		so_object.Invoice.Invoice_details_attributes[fmt.Sprintf("%d", i)]["quantity"], _ = strconv.ParseFloat(po.(map[string]interface{})["booked_quantity"].(string), 32)
		so_object.Invoice.Invoice_details_attributes[fmt.Sprintf("%d", i)]["item_code"] = po.(map[string]interface{})["item"].(map[string]interface{})["code"].(string)
		so_object.Invoice.Invoice_details_attributes[fmt.Sprintf("%d", i)]["unit_price"] = 1.00
		row_table += fmt.Sprintf(
			`<tr>
							<th class="tg-yw4l">%s</th>
							<th class="tg-yw4l">%s</th>
							<th class="tg-yw4l">%s</th>
						</tr>`,
			po.(map[string]interface{})["item"].(map[string]interface{})["code"].(string),
			po.(map[string]interface{})["item"].(map[string]interface{})["name"].(string),
			po.(map[string]interface{})["booked_quantity"].(string),
		)
	}

	// Parsing sale order objecto to json
	so_json, err := json.Marshal(so_object)

	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"json.Marshal. %s"}`, err.Error()), StatusCode: 500}, nil
	}

	// Configuring post reques to create the new sale order
	url_so := "https://zauru.herokuapp.com/sales/orders.json"
	req, err = http.NewRequest("POST", url_so, bytes.NewBuffer(so_json))
	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"http.NewRequest.so. %s"}`, err.Error()), StatusCode: 500}, nil
	}
	req.Header.Set("X-User-Email", request.Headers["X-User-Email-2"])
	req.Header.Set("X-User-Token", request.Headers["X-User-Token-2"])
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-type", "application/json")

	client = &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"client.Do.so. %s"}`, err.Error()), StatusCode: 500}, nil
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"ioutil.ReadAll.so. %s"}`, err.Error()), StatusCode: 500}, nil
	}

	// Parsing json PO response to object
	var new_so_object interface{}

	if err := json.Unmarshal(body, &new_so_object); err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"Invalid created sale order json response. %s"}`, err.Error()), StatusCode: 500}, nil
	}

	sale_order := po_object.(map[string]interface{})

	// Configuring SQS
	svc := sqs.New(session.New(), &aws.Config{Region: aws.String("us-west-2")})

	// URL to our queue
	// TODO replace by lambda environment variable
	qURL := os.Getenv("URL_QUEUE_AUTOMATOR_MAILER")

	// Building html body
	body_html := fmt.Sprintf(`	
					<p>Se ha generado una nueva orden de venta con el siguiente detalle:<p>
					<style type="text/css">
						.tg  {border-collapse:collapse;border-spacing:0;border-color:#aabcfe;margin:0px auto;}
						.tg td{font-family:Arial, sans-serif;font-size:14px;padding:10px 5px;border-style:solid;border-width:1px;overflow:hidden;word-break:normal;border-color:#aabcfe;color:#669;background-color:#e8edff;}
						.tg th{font-family:Arial, sans-serif;font-size:14px;font-weight:normal;padding:10px 5px;border-style:solid;border-width:1px;overflow:hidden;word-break:normal;border-color:#aabcfe;color:#039;background-color:#b9c9fe;}
						.tg .tg-yw4l{vertical-align:top}
					</style>
					<table class="tg">
						%s
					</table>
					<p>Ir a la orden de venta <a href='https://zauru.herokuapp.com/sales/orders/%f'></a> <p>`,
		row_table,
		sale_order["id"].(float64),
	)

	// Building json body
	message_body := fmt.Sprintf(
		`{"title":"%s","body":"%s","recipient_email":"%s","entity_logo":"%s","entity_name":"%s","recipient_name":"%s","sender_name":"%s","sender_email":"%s","extra_cc":"%s","extra_bcc":"%s","attached_file_url":"","attachment_name":""}`,
		params.Email_title,
		body_html,
		params.Recipient_email,
		params.Email_entity_logo,
		params.Email_entity_name,
		params.Email_recipient_name,
		purchase_order["agency"].(map[string]interface{})["notes"].(string),
		params.Sender_email,
		params.Email_extra_cc,
		params.Email_extra_bcc,
	)

	// Sending SQS message
	result, err := svc.SendMessage(&sqs.SendMessageInput{
		DelaySeconds: aws.Int64(10),
		MessageBody:  aws.String(message_body),
		QueueUrl:     &qURL,
	})

	if err != nil {
		return events.APIGatewayProxyResponse{Body: fmt.Sprintf(`{error:"svc.SendMessage. %s"}`, err.Error()), StatusCode: 500}, nil
	}

	return events.APIGatewayProxyResponse{Body: string(body) + *result.MessageId, StatusCode: 200}, nil
}

func main() {
	lambda.Start(Handler)
}
