package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
)

var (
	customerName = flag.String("customerName", "", "Customer Name (financialtimes)")
	userName     = flag.String("userName", "", "User name (georgeangel)")
	password     = flag.String("password", "", "Password")
	host         = flag.String("host", "", "Host to register as the tunnel (aws.elb.com)")
	fqdn         = flag.String("fqdn", "", "Hostname to register (*.ft.com)")
	zone         = flag.String("zone", "", "Dyn zone (ft.com)")
)

type dynClient interface {
	session() (string, error)
	cnameRecord() (string, error)
	createCNAMERecord() error
	updateCNAMERecord(record *string) error
	publish() error
}

type dynHTTPClient struct {
	httpClient *http.Client
	token      string
}

func (dynClient *dynHTTPClient) session() (string, error) {
	body := map[string]string{"customer_name": *customerName, "user_name": *userName, "password": *password}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", "https://api.dynect.net/REST/Session/", bytes.NewReader(jsonBody))
	req.Header.Add("Content-Type", `application/json`)
	resp, err := dynClient.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(resp.Body)
	respMap := map[string]interface{}{}
	err = decoder.Decode(&respMap)
	if err != nil {
		return "", err
	}
	if respMap["status"] != "success" {
		return "", errors.New(fmt.Sprintf("dyn: login was not successful: %v", respMap["msgs"]))
	}
	fmt.Printf("Resp map: %v\n", respMap)
	data := respMap["data"].(map[string]interface{})
	return data["token"].(string), nil
}

func (dynClient *dynHTTPClient) cnameRecord() (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.dynect.net/REST/CNAMERecord/%s/%s/", *zone, *fqdn), nil)
	req.Header.Add("Content-Type", `application/json`)
	req.Header.Add("Auth-Token", dynClient.token)
	resp, err := dynClient.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(resp.Body)
	respMap := map[string]interface{}{}
	err = decoder.Decode(&respMap)
	if err != nil {
		return "", err
	}
	//fmt.Printf("respMap: %v\n", respMap)
	if respMap["status"] != "success" {
		msgsSlice := respMap["msgs"].([]interface{})
		for _, msgsI := range msgsSlice {
			msgs := msgsI.(map[string]interface{})
			if msgs["ERR_CD"].(string) == "NOT_FOUND" {
				return msgs["ERR_CD"].(string), nil
			}
		}
		return "", errors.New(fmt.Sprintf("dyn: could not get fqdn: %v", respMap))
	}
	data := respMap["data"].([]interface{})
	if len(data) != 1 {
		return "", errors.New(fmt.Sprintf("dyn: did not find exactly 1 record: %v", data))
	}
	return data[0].(string), nil
}

func (dynClient *dynHTTPClient) createCNAMERecord() error {
	rdata := map[string]string{"cname": *host}
	body := map[string]interface{}{"rdata": rdata, "ttl": 600}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("https://api.dynect.net/REST/CNAMERecord/%s/%s/", *zone, *fqdn), bytes.NewReader(jsonBody))
	req.Header.Add("Content-Type", `application/json`)
	req.Header.Add("Auth-Token", dynClient.token)
	resp, err := dynClient.httpClient.Do(req)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(resp.Body)
	respMap := map[string]interface{}{}
	err = decoder.Decode(&respMap)
	if err != nil {
		return err
	}
	if respMap["status"] != "success" {
		return errors.New(fmt.Sprintf("dyn: Failed to create CNAME: %v", respMap["msgs"]))
	}
	return nil
}

func (dynClient *dynHTTPClient) updateCNAMERecord(record *string) error {
	rdata := map[string]string{"cname": *host}
	body := map[string]interface{}{"rdata": rdata, "ttl": 600}
	fmt.Printf("Body: %v\n", body)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	fmt.Printf("Record: %s\n", *record)
	req, err := http.NewRequest("PUT", fmt.Sprintf("https://api.dynect.net%s", *record), bytes.NewReader(jsonBody))
	req.Header.Add("Content-Type", `application/json`)
	req.Header.Add("Auth-Token", dynClient.token)
	resp, err := dynClient.httpClient.Do(req)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(resp.Body)
	respMap := map[string]interface{}{}
	err = decoder.Decode(&respMap)
	if err != nil {
		return err
	}
	fmt.Printf("Update response: %v\n", respMap)
	if respMap["status"] != "success" {
		return errors.New(fmt.Sprintf("dyn: Failed to update CNAME: %v", respMap["msgs"]))
	}
	return nil
}

func (dynClient *dynHTTPClient) publish() error {
	body := map[string]string{"publish": "true"}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", fmt.Sprintf("https://api.dynect.net/REST/Zone/%s/", *zone), bytes.NewReader(jsonBody))
	req.Header.Add("Content-Type", `application/json`)
	req.Header.Add("Auth-Token", dynClient.token)
	resp, err := dynClient.httpClient.Do(req)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(resp.Body)
	respMap := map[string]interface{}{}
	err = decoder.Decode(&respMap)
	if err != nil {
		return err
	}
	if respMap["status"] != "success" {
		return errors.New(fmt.Sprintf("dyn: Failed publish: %v", respMap["msgs"]))
	}
	fmt.Printf("Updated CNAME for %s to %s\n", *fqdn, *host)
	return nil
}

func main() {
	flag.Parse()
	dynClient := &dynHTTPClient{httpClient: &http.Client{}}
	token, err := dynClient.session()
	if err != nil {
		panic(err)
	}
	dynClient.token = token
	currentFQDN, err := dynClient.cnameRecord()
	if err != nil {
		panic(err)
	}
	fmt.Printf("currentFQDN: %s\n", currentFQDN)
	if currentFQDN == "NOT_FOUND" {
		err = dynClient.createCNAMERecord()
		if err != nil {
			panic(err)
		}
	} else {
		err = dynClient.updateCNAMERecord(&currentFQDN)
		if err != nil {
			panic(err)
		}
	}
	err = dynClient.publish()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Succesfully published\n")
}
