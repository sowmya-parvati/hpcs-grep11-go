/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package examples

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/IBM-Cloud/hpcs-grep11-go/v2/pkg/authorize"
	"github.com/IBM-Cloud/hpcs-grep11-go/v2/pkg/util"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	// ClientConfig contains required information to connect to a remote HPCS instance
	ClientConfig util.ClientConfig
)

// Obtain Address, APIKey, and IAMEndpoint environment variables if they are set.
// The Address and APIKey variables can be changed prior to running the sample program.
// Optionally, GREP11_ADDRESS, GREP11_APIKEY and GREP11_IAMENDPOINT environment variables can be set,
// overriding the current/default values of the ClientConfig fields: Address, APIKey and IAMEndpoint.
//
// Two independent connection modes are supported:
//
// Mode 1 — mTLS / on-prem (set GREP11_ONPREM=1):
//
//	Only GREP11_ADDRESS is used. APIKey and IAMEndpoint are not needed.
//	The following env vars are required in this mode:
//	  GREP11_CLIENT_CERT — path to the client certificate PEM file
//	  GREP11_CLIENT_KEY  — path to the client private key file
//	  GREP11_CA_CERT     — path to the CA certificate PEM file
//
// Mode 2 — IAM / IBM Cloud (default, GREP11_ONPREM not set):
//
//	GREP11_ADDRESS, GREP11_APIKEY, and GREP11_IAMENDPOINT are used.
//	Set GREP11_LOCAL to bypass TLS (insecure local connections).
func init() {

	// Address is shared by both modes.
	ClientConfig.Address = "<grep11_server_address>:<port>"
	if address, exists := os.LookupEnv("GREP11_ADDRESS"); exists {
		ClientConfig.Address = address
	}

	if _, onprem := os.LookupEnv("GREP11_ONPREM"); onprem {
		// ── mTLS / on-prem mode ──────────────────────────────────────────────
		// APIKey and IAMEndpoint are not used in this mode.
		// Cert/key/CA paths must be supplied via environment variables.
		clientCertPath, exists := os.LookupEnv("GREP11_CLIENT_CERT")
		if !exists {
			panic(fmt.Errorf("GREP11_CLIENT_CERT must be set when GREP11_ONPREM is enabled"))
		}
		clientKeyPath, exists := os.LookupEnv("GREP11_CLIENT_KEY")
		if !exists {
			panic(fmt.Errorf("GREP11_CLIENT_KEY must be set when GREP11_ONPREM is enabled"))
		}
		caCertPath, exists := os.LookupEnv("GREP11_CA_CERT")
		if !exists {
			panic(fmt.Errorf("GREP11_CA_CERT must be set when GREP11_ONPREM is enabled"))
		}

		certificate, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			panic(fmt.Errorf("load client cert error: %v", err))
		}

		caCert, err := ioutil.ReadFile(caCertPath)
		if err != nil {
			panic(fmt.Errorf("read CA error: %v", err))
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caCert) {
			panic(fmt.Errorf("failed to append CA certificate"))
		}

		// ServerName must match the SAN in the server certificate.
		// ClientConfig.Address is "host:port" and the cert SAN includes the port,
		// so pass the full address as ServerName to satisfy TLS verification.
		ClientConfig.DialOpts = []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				Certificates: []tls.Certificate{certificate},
				RootCAs:      certPool,
				ServerName:   ClientConfig.Address,
			})),
		}
	} else {
		// ── IAM / IBM Cloud mode ─────────────────────────────────────────────
		ClientConfig.APIKey = "<ibm_cloud_apikey>"
		ClientConfig.IAMEndpoint = "https://iam.cloud.ibm.com"

		if apiKey, exists := os.LookupEnv("GREP11_APIKEY"); exists {
			ClientConfig.APIKey = apiKey
		}
		if iamEndpoint, exists := os.LookupEnv("GREP11_IAMENDPOINT"); exists {
			ClientConfig.IAMEndpoint = iamEndpoint
		}

		ClientConfig.DialOpts = []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
			grpc.WithPerRPCCredentials(&authorize.IAMPerRPCCredentials{
				APIKey:   ClientConfig.APIKey,
				Endpoint: ClientConfig.IAMEndpoint,
			}),
		}

		if _, exists := os.LookupEnv("GREP11_LOCAL"); exists {
			ClientConfig.DialOpts = []grpc.DialOption{
				grpc.WithInsecure(),
				grpc.WithBlock(),
			}
		}
	}
}
