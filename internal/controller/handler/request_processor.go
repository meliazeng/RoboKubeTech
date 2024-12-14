package handler

import (
	"bytes"
	"context"
	//"encoding/json"
	"fmt"
	"io"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"strings"
	//robokubev1 "meliazeng/RoboKube/api/v1"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RequestHandler struct {
	Payload       *apiextensions.JSON
	RequestMethod string
	RequestUrl    string
	ResourceType  ResourceType
	TargetName    string
	ErrorCondtion bool
	DryRun        bool
}

type RequestParams struct {
	ResourceType  ResourceType
	TargetName    string
	ErrorCondtion bool
	DryRun        bool
	// reserve for credential etc...
}

func NewRequestHandler(payload *apiextensions.JSON, url string, action string, paras RequestParams) *RequestHandler {
	return &RequestHandler{
		Payload:       payload,
		ResourceType:  paras.ResourceType,
		TargetName:    paras.TargetName,
		ErrorCondtion: paras.ErrorCondtion,
		RequestMethod: action,
		RequestUrl:    url,
		DryRun:        paras.DryRun,
	}
}

func (p *RequestHandler) Handle(ctx context.Context, client client.Client, obj client.Object, f func(client.Object)) (*Result, error) {
	/* Request type:
	prepare the payload
	get the url (service of the pod)
	1. Get -- get status of the pod /status: that is for entity to call pod
	2. Post --- seend request to /task:  that is for task to call pod

	so obj will be entity where the url can be getting from status
	the return will be the condition for task or entity's condition

	handle the response
	*/
	// Create the HTTP request
	var (
		req *http.Request
		err error
	)
	if strings.ToUpper(p.RequestMethod) != http.MethodGet {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, p.RequestUrl, bytes.NewReader(p.Payload.Raw))
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, p.RequestUrl, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if !p.DryRun {
		// Send the HTTP request
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		// Get the HTTP status code
		statusCode := resp.StatusCode

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		// Now you have the status code in the 'statusCode' variable
		if statusCode != http.StatusOK {
			// Handle non-200 status codes here
			return nil, fmt.Errorf("request failed with status code: %d, body: %s", statusCode, body)
		}
		return &Result{
			Status:       "Success",
			Reason:       "Get response successful",
			Message:      string(body),
			ResourceName: p.TargetName,
			ResourceType: p.ResourceType,
		}, nil
	} else {
		return &Result{
			Status:       "Success",
			Reason:       "If you have gone this far, dryrun is good",
			Message:      "{}",
			ResourceName: p.TargetName,
			ResourceType: p.ResourceType,
		}, nil
	}
}

var _ Handler[Result] = &RequestHandler{}
