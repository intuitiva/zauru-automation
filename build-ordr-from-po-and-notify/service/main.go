package main

import (
	"os"
	"fmt"
	"log"
	"time"
	"strings"
	"runtime"
    "github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"encoding/json"
	"net/http"
	"bytes"
	"io/ioutil"
	"strconv"
	"github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type apiError struct {
	code string
	msg string
	err string
}

func (e *apiError) Error() string {
	pc := make([]uintptr, 10)  // at least 1 entry needed
	runtime.Callers(2, pc)
	f := runtime.FuncForPC(pc[0])
	_, line := f.FileLine(pc[0])
	log.Print(fmt.Sprintf(`{"code":"%s","msg":"%s","error":"%s:%d %s"}`, e.code, e.msg, f.Name(), line, e.err))
	return fmt.Sprintf(`{"code":"%s","msg":"%s"}`, e.code, e.msg)
}

func (e *apiError) New(code string, msg string, err string) error {
	return &apiError{code, msg, err}
}

var errors = &apiError{}

type emailInfo struct {
	Recipient string
	Title string
	Recipient_name string
	Sender_name string
	Sender string
	Extra_cc string
	Extra_bcc string
	Entity_name string
	Entity_id int
	Entity_logo string
}

type RequestParams struct {
	Purchase_order_id int
	Payment_term_id int
	Seller_id int
	Payee_id int
	Agency_id int
	Dispatcher emailInfo
	Applicant emailInfo
}



type poData struct {
	Reference string `json:"reference"`
	Memo string `json:"memo"`
	Date string `json:"date"`
	Taxable bool `json:"taxable"`
	Pos bool `json:"pos"`
	Payee_id int `json:"payee_id"`
	Payment_term_id int `json:"payment_term_id"`
	Seller_id int `json:"seller_id"`
	Agency_id int `json:"agency_id"`
	Invoice_details_attributes map[string] map[string]interface{} `json:"invoice_details_attributes"`
}

type poStruct struct {
	Invoice poData `json:"invoice"`
}

type response events.APIGatewayProxyResponse

// This function make validations and return body params
func getParams(request * events.APIGatewayProxyRequest) (*RequestParams, error) {
	if request.Headers["X-User-Email-1"] == "" {
		return nil, errors.New("405", `user email (1) is missing.`, "")
	}

	if request.Headers["X-User-Token-1"] == "" {
		return nil, errors.New("405", `user token (1) is missing.`, "")
	}

	if request.Headers["X-User-Email-2"] == "" {
		return nil, errors.New("405", `user email (2) is missing.`, "")
	}

	if request.Headers["X-User-Token-2"] == "" {
		return nil, errors.New("405", `user token (2) is missing.`, "")
	}

	var params RequestParams
	
	if err := json.Unmarshal([]byte(request.Body), &params); err != nil {
		return nil, errors.New("406", "parsing params error.", err.Error())
	}
	
	if params.Purchase_order_id == 0{
		return nil, errors.New("405", `purchase order id is missing.`, "")
	}

	if params.Payment_term_id == 0{
		return nil, errors.New("405", `payment term id is missing.`, "")
	}

	if params.Seller_id == 0{
		return nil, errors.New("405", `seller id is missing.`, "")
	}

	if params.Payee_id == 0{
		return nil, errors.New("405", `payee id is missing.`, "")
	}

	if params.Agency_id == 0{
		return nil, errors.New("405", `agency id is missing.`, "")
	}

	if params.Dispatcher.Recipient == ""{
		return nil, errors.New("405", `dispatcher recipient email is missing.`, "")
	}

	if params.Dispatcher.Title == ""{
		return nil, errors.New("405", `dispatcher email title is missing.`, "")
	}
		
	if params.Dispatcher.Recipient_name == ""{
		return nil, errors.New("405", `dispatcher email recipient name is missing.`, "")
	}
	
	if params.Applicant.Recipient == ""{
		return nil, errors.New("405", `applicant recipient email is missing.`, "")
	}

	if params.Applicant.Title == ""{
		return nil, errors.New("405", `applicant email title is missing.`, "")
	}
		
	if params.Applicant.Recipient_name == ""{
		return nil, errors.New("405", `applicant email recipient name is missing.`, "")
	}

	return &params, nil
}

func httpRequest(url string, method string, params []byte, headers map[string]string) (interface{}, error) {
	log.Print(url)
	log.Print(method)
	log.Print(string(params))
	
	// Configuring http request
	req, err := http.NewRequest(method, url, bytes.NewBuffer(params))
	if err != nil {
		return nil, errors.New("502", "Internal Error", err.Error())
	}
	// Add header values
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("503", "Internal Error", err.Error())
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, errors.New("504", "Internal Error", err.Error())
	}
	// Parsing json response to object
	var data_object interface{}
	err = json.Unmarshal(body, &data_object);
	if err != nil {
		log.Print(string(body))
		return nil, errors.New("505", "Internal Error", err.Error())
	}
	
	return data_object, nil
}

func sendToQueue( info emailInfo, order_id float64, order_number string, order_url string, agency_name string, detail_message string ) ( *sqs.SendMessageOutput, error ) {
	// Configuring SQS
	svc := sqs.New(session.New(), &aws.Config{Region: aws.String("us-west-2")})

	// URL to our queue
	qURL := os.Getenv("URL_QUEUE_AUTOMATOR_MAILER")

	// Building html body
	footer_message := ""
	if order_id == 0 {
		footer_message = "<p> Nota: La orden de venta no se generó correctamente, debido a que no hay existencias suficientes para uno o varios de los productos.<p>"
	} else {
		footer_message = fmt.Sprintf(`	<center>
											<a href='%s%s%.f' class='button'>Ir a Orden %s</button>
										</center>`, os.Getenv("URL_ZAURU"), order_url, order_id, order_number)
	}
	body_html := fmt.Sprintf(`
					<style type='text/css'>
					.tg  {border-collapse:collapse;border-spacing:0;border-color:#999;margin:0px auto;}
					.tg td.odd{font-family:Arial, sans-serif;font-size:14px;padding:10px 5px;border-style:solid;border-width:0px;overflow:hidden;word-break:normal;border-color:#999;color:#444;background-color:#c4d9f3;}
					.tg td{font-family:Arial, sans-serif;font-size:14px;padding:10px 5px;border-style:solid;border-width:0px;overflow:hidden;word-break:normal;border-color:#999;color:#444;background-color:#ecf5ff;}
					.tg th{font-family:Arial, sans-serif;font-size:14px;font-weight:normal;padding:10px 5px;border-style:solid;border-width:0px;overflow:hidden;word-break:normal;border-color:#999;color:#fff;background-color:#26ADE4;}
					.tg .tg-0pky{border-color:inherit;text-align:left;vertical-align:top}
					.button {border: 1px solid #74a0b9;background: #65a9d7;padding: 10.5px 21px;-webkit-border-radius: 6px;-moz-border-radius: 6px;border-radius: 6px;-webkit-box-shadow: rgba(255,255,255,0.4) 0 1px 0, inset rgba(255,255,255,0.4) 0 1px 0;-moz-box-shadow: rgba(255,255,255,0.4) 0 1px 0, inset rgba(255,255,255,0.4) 0 1px 0;box-shadow: rgba(255,255,255,0.4) 0 1px 0, inset rgba(255,255,255,0.4) 0 1px 0;text-shadow: #7ea4bd 0 1px 0;color: #ffffff;font-size: 14px;font-family: helvetica, serif;text-decoration: none;vertical-align: middle;}
					</style>
					<p>Se ha generado una nueva orden desde tienda con el siguiente detalle:<p>
					<table class='tg'>
						<tr>
							<th class='tg-us36'>Cantidad</th>
							<th class='tg-us36'>Código</th>
							<th class='tg-us36'>Nombre</th>
						</tr>
						%s
					</table>
					<br><br><br><br><br>%s`,
					detail_message,
					footer_message,
				)

	// Building json body
	message_body := fmt.Sprintf(
		`{"id":"NOTIFICATION%.f%d","template_name":"automator","entity_id":%d,"title":"%s %s %s","body":"%s","recipient_email":"%s","entity_logo":"%s","entity_name":"%s","recipient_name":"%s","sender_name":"%s","sender_email":"%s","extra_cc":"%s","extra_bcc":"%s"}`,
		order_id,
		int32(time.Now().Unix()),
		info.Entity_id,
		info.Title,
		agency_name,
		order_number,
		strings.Replace(strings.Replace(body_html,"\n", "", -1), "\t", "", -1),
		info.Recipient,
		info.Entity_logo,
		info.Entity_name,
		info.Recipient_name,
		agency_name,
		info.Sender,
		info.Extra_cc,
		info.Extra_bcc,
	)

	// Sending SQS message
	return svc.SendMessage(&sqs.SendMessageInput{
        DelaySeconds: aws.Int64(10),
        MessageBody: aws.String(message_body),
        QueueUrl:    &qURL,
	})
}

func Handler(request events.APIGatewayProxyRequest) (response, error) {
	// Validate if api key and user email is not empty

	params, err := getParams(&request)

	if err != nil {
		return response {Body: err.Error(), StatusCode: 400}, nil
	}

	// PO request setup
	headers_po := make(map[string] string)
	url_po := fmt.Sprintf("https://app.zauru.com/purchases/purchase_orders/%d.json", params.Purchase_order_id)
	headers_po["X-User-Email"] = request.Headers["X-User-Email-1"]
	headers_po["X-User-Token"] = request.Headers["X-User-Token-1"]

	// Send request, getting response object
	po_object, err := httpRequest(url_po, "GET", []byte(""), headers_po)
	if(err != nil){
		return response {Body: err.Error(), StatusCode: 500}, nil
	}

	purchase_order := po_object.(map[string] interface{})


	// Filling sale order data
	so_data := &poData{
		Reference: purchase_order["agency"].(map[string] interface{})["name"].(string),
		Memo: purchase_order["memo"].(string),
		Date: purchase_order["issue_date"].(string),
		Taxable: true,
		Pos: false,
		Payee_id: params.Payee_id,
		Payment_term_id: params.Payment_term_id,
		Seller_id: params.Seller_id,
		Agency_id: params.Agency_id,
		Invoice_details_attributes: make(map[string] map[string]interface{}),
	}

	so_object := &poStruct{
		Invoice: *so_data,
	}

	row_table := ""
	for i, po:= range purchase_order["purchase_order_details"].([] interface{}) {
		so_object.Invoice.Invoice_details_attributes[fmt.Sprintf("%d",i)] = make(map[string]interface{})
		so_object.Invoice.Invoice_details_attributes[fmt.Sprintf("%d",i)]["quantity"], _ = strconv.ParseFloat(po.(map[string]interface{})["booked_quantity"].(string), 32)
		so_object.Invoice.Invoice_details_attributes[fmt.Sprintf("%d",i)]["item_code"] = po.(map[string]interface{})["item"].(map[string]interface{})["code"].(string)
		so_object.Invoice.Invoice_details_attributes[fmt.Sprintf("%d",i)]["unit_price"] = 1.00
		is_odd := ""
		if i % 2 != 0{
			is_odd = "odd"
		}
		quantity, _ := strconv.ParseFloat(po.(map[string]interface{})["booked_quantity"].(string), 32)
		row_table += fmt.Sprintf(
						`<tr>
							<td class='tg-yw4l %s'>%.f</th>
							<td class='tg-yw4l %s'>%s</th>
							<td class='tg-yw4l %s'>%s</th>
						</tr>`,
						is_odd,
						quantity,
						is_odd,
						po.(map[string]interface{})["item"].(map[string]interface{})["code"].(string),
						is_odd,
						po.(map[string]interface{})["item"].(map[string]interface{})["name"].(string),
		)
	}

	// Parsing sale order objecto to json
	so_json, err := json.Marshal(so_object)
	if err != nil {
		return response {Body: errors.New("506", "Internal Error", err.Error()).Error(), StatusCode: 500}, nil
	}

	var sale_order map[string] interface{}
	var sale_order_id float64
	var sale_order_number string

	// SO request setup
	headers_so := make(map[string] string)
	url_so := "https://app.zauru.com/sales/orders.json"
	headers_so["X-User-Email"] = request.Headers["X-User-Email-2"]
	headers_so["X-User-Token"] = request.Headers["X-User-Token-2"]
	headers_so["Accept"] = "application/json"
	headers_so["Content-type"] = "application/json"
	new_so_object, err := httpRequest(url_so, "POST", so_json, headers_so)

	if err != nil {
		sale_order_id = 0
		sale_order_number = ""
	} else {
		sale_order = new_so_object.(map[string] interface{})
		sale_order_id = sale_order["id"].(float64)
		sale_order_number = sale_order["order_number"].(string)
	}

	// Sending to applicant
	result, err := sendToQueue( params.Applicant, purchase_order["id"].(float64), purchase_order["id_number"].(string), "/purchases/purchase_orders/", purchase_order["agency"].(map[string] interface{})["name"].(string), row_table)

	warning := ""

	if err == nil {
		log.Print(fmt.Sprintf(`{"target": "applicant" ,"sqs_status":"sended","sqs_id":"%s"}`,*result.MessageId))
	} else {
		warning += errors.New("507", "Internal Error", err.Error()).Error()
	}

	// Sending to dispatcher
	result, err = sendToQueue( params.Dispatcher, sale_order_id, sale_order_number, "/sales/orders/", purchase_order["agency"].(map[string] interface{})["name"].(string), row_table)

	if err == nil {
		log.Print(fmt.Sprintf(`{"target": "dispatcher" ,"sqs_status":"sended","sqs_id":"%s"}`,*result.MessageId))
	} else {
		warning += errors.New("508", "Internal Error", err.Error()).Error()
	}
	
	return response {Body: `{"code":"200","message":"successfully processed.","warning":%s}`, StatusCode: 201}, nil
}

func main() {
	lambda.Start(Handler)
}