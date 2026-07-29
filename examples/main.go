/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package examples

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"

	"github.com/IBM-Cloud/hpcs-grep11-go/v2/pkg/authorize"
	"github.com/IBM-Cloud/hpcs-grep11-go/v2/pkg/util"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// fixedServerNameCreds wraps a tls.Config and performs the TLS handshake
// directly, bypassing gRPC's ClientHandshake which always strips the port from
// the authority and overwrites ServerName. This allows a ServerName containing
// a port (e.g. "grep11.example.com:9876") to be used as-is for TLS verification,
// matching a non-standard cert SAN that includes the port.
type fixedServerNameCreds struct {
	cfg *tls.Config
}

func (f fixedServerNameCreds) ClientHandshake(ctx context.Context, _ string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	// Perform TLS handshake directly with our config — ServerName is preserved.
	conn := tls.Client(rawConn, f.cfg)
	if err := conn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, credentials.TLSInfo{State: conn.ConnectionState()}, nil
}

func (f fixedServerNameCreds) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return conn, nil, nil
}

func (f fixedServerNameCreds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "tls"}
}

func (f fixedServerNameCreds) Clone() credentials.TransportCredentials {
	return fixedServerNameCreds{cfg: f.cfg.Clone()}
}

func (f fixedServerNameCreds) OverrideServerName(name string) error {
	f.cfg.ServerName = name
	return nil
}

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

		// The server certificate SAN is "host:port" (non-standard, port included).
		// gRPC's ClientHandshake strips the port from the dial authority and overwrites
		// ServerName — so we use fixedServerNameCreds to suppress that override.
		// This lets Go TLS use the ServerName we set here ("host:port") directly,
		// so standard certificate verification works with InsecureSkipVerify: false.
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{certificate},
			RootCAs:      certPool,
			ServerName:   ClientConfig.Address, // "grep11.example.com:9876" — matches cert SAN
		}
		ClientConfig.DialOpts = []grpc.DialOption{
			grpc.WithTransportCredentials(fixedServerNameCreds{cfg: tlsCfg}),
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
