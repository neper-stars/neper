package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/rs/zerolog"
	"github.com/steinfletcher/apitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/cmd/neper/cmd"
	"github.com/neper-stars/neper/lib/embeddednats"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/testutils/fixtureloader"
)

type APICall struct {
	Method          string
	URL             string
	InputPayload    string
	ResponseStatus  string
	ResponseHeaders string
	ResponseBody    string
}

// JSONObj is a map
type JSONObj map[string]interface{}

// APITester provides facilities to unittest the Carnet rest api
type APITester struct {
	t       *testing.T
	handler http.Handler
	api     *operations.NeperAPI
	capi    *restapi.ConfiguredAPI

	headers map[string]string

	TestLogger *testutils.TestLogger
	Logger     zerolog.Logger
	DB         *database.TestDB

	Config restapi.Config

	fixtureLoader *fixtureloader.Loader

	captureLogs  *captureWriter
	captureCalls []APICall

	now *time.Time
}

type RestAPIConfigUpdate func(restapi.Config) restapi.Config

func NewAPITesterConfigUpdater(t *testing.T, log *zerolog.Logger, autoDelete bool) *APITesterConfigUpdater {
	t.Helper()
	return &APITesterConfigUpdater{
		t:          t,
		log:        log,
		autoDelete: autoDelete,
	}
}

type APITesterConfigUpdater struct {
	t          *testing.T
	log        *zerolog.Logger
	autoDelete bool
}

func (a *APITesterConfigUpdater) UpdateConfig(config restapi.Config) restapi.Config {
	runner := stars.GetTestStarsRunnerWithAutoDelete(a.t, a.log, a.autoDelete)
	config.StarsRunner = runner

	// NATS setup
	// create a test key on the fly for this run
	kp, err := nkeys.CreateUser()
	if err != nil {
		a.t.Fatal(err)
	}
	epk, err := kp.Seed()
	if err != nil {
		a.t.Fatal(err)
	}

	signatureHandler, err := embeddednats.NewClientSigHandler(epk)
	if err != nil {
		a.t.Fatalf("could not start nats signature handler: %s", err.Error())
	}
	ns := embeddednats.NewEmbeddedServer(signatureHandler.PubKey())
	config.NatsServer = ns

	connHandlers := embeddednats.NewConnHandlers(a.log)
	// launch the NATS.io server
	go config.NatsServer.Run()
	// Wait for NATS server to be ready before connecting
	if !config.NatsServer.WaitReady(10 * time.Second) {
		a.t.Fatal("NATS server failed to start within timeout")
	}

	cOpts := nats.Options{
		InProcessServer: ns,
		AllowReconnect:  false,
		MaxReconnect:    2,
		// ClosedCB:             nil,
		DisconnectedErrCB: connHandlers.DisconnectErrHandler,
		ConnectedCB:       connHandlers.ConnHandler,
		// ReconnectedCB:        nil,
		// DiscoveredServersCB:  nil,
		AsyncErrorCB:         connHandlers.ErrHandler,
		Nkey:                 signatureHandler.PubKey(),
		SignatureCB:          signatureHandler.Sign,
		RetryOnFailedConnect: true,
	}

	nc, err := cOpts.Connect()
	if err != nil {
		a.t.Fatalf("failed to start client conn to inprocess nats server: %s", err.Error())
	}
	config.NatsClientConn = nc

	// Initialize the notification service
	config.NotifyService = notify.NewService(nc, a.log)

	// create the turn generator
	config.TurnGenerator = stars.NewTurnGenerator(a.log, config.NatsClientConn, config.DB, config.StarsRunner, config.NotifyService)
	// run in background mode ... shutdown is configured in the restapi/configure_neper.go file
	go config.TurnGenerator.Run(context.Background())
	// wait for the turn generator to be fully initialized before proceeding
	<-config.TurnGenerator.Ready()
	if !config.TurnGenerator.IsRunning() {
		a.t.Fatal("TurnGenerator failed to start")
	}
	config.OnShutdown(config.TurnGenerator.Shutdown)

	return config
}

// NewAPITester sets up a APITester
func NewAPITester(t *testing.T, config ...RestAPIConfigUpdate) *APITester {
	t.Helper()

	testLogger := testutils.NewTestLogger(t)
	var captureLogs = &captureWriter{nil, zerolog.ConsoleWriter{Out: testLogger}}
	log := testLogger.Logger().Output(captureLogs)

	tester := APITester{
		t:           t,
		TestLogger:  testLogger,
		headers:     make(map[string]string),
		Logger:      log,
		captureLogs: captureLogs,
	}

	tester.ResetDB(config...)
	return &tester
}

func (tester *APITester) ResetDB(configUpdate ...RestAPIConfigUpdate) {
	var success bool

	if tester.capi != nil {
		tester.capi.PreServerShutdown()
		tester.capi.ServerShutdown()
	}
	if tester.DB != nil {
		tester.DB.Close()
	}

	// prepare an empty DB
	db := database.GetTestDB(context.TODO(), tester.t, migration.Source)

	defer func() {
		if !success {
			db.Close()
		}
	}()

	tester.DB = db
	tester.fixtureLoader = fixtureloader.NewLoader(tester.t, db.DB, tester.Logger)

	testNow := func() time.Time {
		if tester.now != nil {
			return *tester.now
		}
		return time.Now()
	}

	testTokenOptions := auth.TokenOptions{
		Secret:             "796085333b87c6ec9e35b270861cebf4e6b94611439d01baecada6a81624960c",
		ExpirationOpt:      nil,
		Expiration:         0,
		CacheMaxSize:       0,
		CachePurgeDelay:    0,
		CachePurgeDelayOpt: nil,
	}

	authenticator := auth.NewAuth(testTokenOptions, db.DB, testNow, tester.Logger)

	config := restapi.Config{
		Log:           tester.Logger,
		DB:            db.DB,
		TokenOptions:  testTokenOptions,
		Authenticator: authenticator,

		Now: testNow,
	}

	for _, update := range configUpdate {
		if update != nil {
			config = update(config)
		}
	}

	// shutdown implementation goes here
	config.OnShutdown(func() {})

	serveCmd := cmd.NewServerCmd()

	configuredAPI, err := restapi.ConfigureAPI(serveCmd.API, serveCmd.Server, config)
	require.NoError(tester.t, err)

	defer func() {
		if !success {
			serveCmd.API.ServerShutdown()
		}
	}()

	success = true

	tester.Config = config
	tester.handler = configuredAPI.Handler()
	tester.api = serveCmd.API
	tester.capi = configuredAPI
}

// SetNow changes the 'now' time
func (tester *APITester) SetNow(t time.Time) func() {
	save := tester.now
	tester.now = &t
	return func() {
		tester.now = save
	}
}

// ResetNow sets 'now' back to the actual current time
func (tester *APITester) ResetNow() {
	tester.now = nil
}

type captureWriter struct {
	capture func(string)
	next    io.Writer
}

func (w captureWriter) Write(data []byte) (int, error) {
	if w.capture != nil {
		w.capture(string(data))
	}
	return w.next.Write(data)
}

// CaptureLogs starts capturing the logs and send them to the callback. It
// returns a function to stop the capture
func (tester *APITester) CaptureLogs(capture func(string)) func() {
	var previous = tester.captureLogs.capture
	tester.captureLogs.capture = capture

	return func() {
		tester.captureLogs.capture = previous
	}
}

// Close releases all underlying resources. MUST be called, preferably with defer
func (tester *APITester) Close() {
	tester.capi.PreServerShutdown()
	tester.capi.ServerShutdown()
	tester.DB.Close()
}

func (tester *APITester) StartCallsCapture() {
	tester.captureCalls = make([]APICall, 0)
}

func (tester *APITester) EndCallsCapture() []APICall {
	r := tester.captureCalls
	tester.captureCalls = nil
	return r
}

func (tester *APITester) TestServer() *httptest.Server {
	return httptest.NewServer(tester.handler)
}

// HandleRequest pass a request through the handler
func (tester *APITester) HandleRequest(req *http.Request) *httptest.ResponseRecorder {
	for key, value := range tester.headers {
		req.Header.Set(key, value)
	}
	tester.Logger.Debug().Msgf("headers: %v", req.Header)
	rr := httptest.NewRecorder()
	tester.handler.ServeHTTP(rr, req)

	return rr
}

func (tester *APITester) Do(req *http.Request) (*http.Response, error) {
	var (
		reqBodyBuf    bytes.Buffer
		respHeaderBuf bytes.Buffer
	)

	if req.Body != nil {
		req.Body = TeeReadCloser(req.Body, &reqBodyBuf)
	}

	rr := tester.HandleRequest(req)

	// We disable bodyclose because it gives a false positive here
	response := rr.Result() //nolint:bodyclose
	defer func() { _ = response.Body.Close() }()

	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewBuffer(respBody))

	if err := response.Header.Write(&respHeaderBuf); err != nil {
		return nil, err
	}

	url := req.URL.Path
	if req.URL.RawQuery != "" {
		url += "?" + req.URL.RawQuery
	}
	tester.captureCalls = append(tester.captureCalls, APICall{
		Method:          req.Method,
		URL:             url,
		InputPayload:    reqBodyBuf.String(),
		ResponseStatus:  response.Status,
		ResponseHeaders: respHeaderBuf.String(),
		ResponseBody:    string(respBody),
	})

	return response, nil
}

func (tester *APITester) APITest(name ...string) *apitest.APITest {
	var (
		reqBodyBuf    bytes.Buffer
		respHeaderBuf bytes.Buffer
	)

	return apitest.New(name...).
		Intercept(func(req *http.Request) {
			for key, value := range tester.headers {
				req.Header.Set(key, value)
			}
			if req.Body != nil {
				req.Body = TeeReadCloser(req.Body, &reqBodyBuf)
			}
		}).
		Observe(func(res *http.Response, req *http.Request, apiTest *apitest.APITest) {
			respBody, err := io.ReadAll(res.Body)
			if err != nil {
				tester.Logger.Err(err).Msg("could not read response body")
			}
			res.Body = io.NopCloser(bytes.NewBuffer(respBody))

			if err := res.Header.Write(&respHeaderBuf); err != nil {
				tester.Logger.Err(err).Msg("could not read response headers")
			}

			url := req.URL.Path
			if req.URL.RawQuery != "" {
				url += "?" + req.URL.RawQuery
			}
			tester.captureCalls = append(tester.captureCalls, APICall{
				Method:          req.Method,
				URL:             url,
				InputPayload:    reqBodyBuf.String(),
				ResponseStatus:  res.Status,
				ResponseHeaders: respHeaderBuf.String(),
				ResponseBody:    string(respBody),
			})
		}).
		Handler(tester.handler)
}

// DoRequest creates and run request
func (tester *APITester) DoRequest(verb, url, body string) (int, string, error) {
	var payload io.Reader
	if len(body) != 0 {
		payload = bytes.NewBufferString(body)
	}
	req, _ := http.NewRequest(verb, url, payload)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// We disable bodyclose because it gives a false positive here
	response, _ := tester.Do(req)
	defer func() {
		assert.NoError(tester.t, response.Body.Close())
	}()
	b, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, "", err
	}
	tester.Logger.Trace().Msg("Get response: " + string(b))
	return response.StatusCode, string(b), nil
}

// MustDoRequest ...
func (tester *APITester) MustDoRequest(verb, url, body string) (int, string) {
	code, body, err := tester.DoRequest(verb, url, body)
	if err != nil {
		tester.t.Fatal(err)
	}
	return code, body
}

// DoJSONRequest creates and run request
func (tester *APITester) DoJSONRequest(verb, url string, body interface{}, response interface{}) (int, error) {
	var sbody string
	if body != nil {
		if s, ok := body.(string); ok {
			sbody = s
		} else {
			b, err := json.Marshal(body)
			if err != nil {
				return 0, err
			}
			sbody = string(b)
		}
	}
	code, s, err := tester.DoRequest(verb, url, sbody)
	if err != nil {
		return 0, err
	}
	if response != nil && len(s) != 0 {
		if err := json.Unmarshal([]byte(s), response); err != nil {
			// attempt to unmarshal an error
			var errorResponse models.Error
			if err := json.Unmarshal([]byte(s), &errorResponse); err == nil {
				return 0, errorResponse
			}
			return 0, err
		}
	}
	return code, nil
}

// MustDoJSONRequest creates and run request that takes no payload
func (tester *APITester) MustDoJSONRequest(verb, url string, body interface{}, response interface{}) int {
	code, err := tester.DoJSONRequest(verb, url, body, response)
	if err != nil {
		tester.t.Fatal(err)
	}
	return code
}

// DoWithPayload creates and run request that takes a payload
func (tester *APITester) DoWithPayload(verb, url string) {

}

// Get runs a GET request
func (tester *APITester) Get(url string) (int, string, error) {
	return tester.DoRequest("GET", url, "")
}

// GetJSON runs a GET request and parse the result as JSON
func (tester *APITester) GetJSON(url string, obj interface{}) (int, error) {
	return tester.DoJSONRequest("GET", url, nil, obj)
}

// MustGet runs a GET request an panics if it fails
func (tester *APITester) MustGet(url string) (int, string) {
	return tester.MustDoRequest("GET", url, "")
}

// MustGetJSON runs a GET request, parse the result as JSON and panic on any error
func (tester *APITester) MustGetJSON(url string, obj interface{}) int {
	return tester.MustDoJSONRequest("GET", url, nil, obj)
}

// Post runs a POST request
func (tester *APITester) Post(url string, body string) (int, string, error) {
	return tester.DoRequest("POST", url, body)
}

// MustPost runs a POST request and panic if it fails
func (tester *APITester) MustPost(url string, body string) (int, string) {
	return tester.MustDoRequest("POST", url, body)
}

// PostJSON runs a POST request with given input marshalled as JSON and responsed
// unmarshalled from JSON
func (tester *APITester) PostJSON(url string, body interface{}, response interface{}) (int, error) {
	return tester.DoJSONRequest("POST", url, body, response)
}

// MustPostJSON is like PostJSON but stops the test on failure
func (tester *APITester) MustPostJSON(url string, body interface{}, response interface{}) int {
	return tester.MustDoJSONRequest("POST", url, body, response)
}

// Put runs a POST request
func (tester *APITester) Put(url string, body string) (int, string, error) {
	return tester.DoRequest("PUT", url, body)
}

// MustPut runs a POST request and panic if it fails
func (tester *APITester) MustPut(url string, body string) (int, string) {
	return tester.MustDoRequest("PUT", url, body)
}

// PutJSON runs a POST request with given input marshalled as JSON and responsed
// unmarshalled from JSON
func (tester *APITester) PutJSON(url string, body interface{}, response interface{}) (int, error) {
	return tester.DoJSONRequest("PUT", url, body, response)
}

// MustPutJSON is like PutJSON but stops the test on failure
func (tester *APITester) MustPutJSON(url string, body interface{}, response interface{}) int {
	return tester.MustDoJSONRequest("PUT", url, body, response)
}

// SetHeader sets a header for all subsequent requests
func (tester *APITester) SetHeader(key, value string) {
	if value != "" {
		tester.headers[key] = value
	} else {
		delete(tester.headers, key)
	}
}

// Login authenticates the user for the subsequent requests
func (tester *APITester) Login(credentials models.Credentials) {
	var token string
	require.Equal(tester.t, 200,
		tester.MustPostJSON("/api/v1/auth/authenticate", credentials, &token))

	tester.SetHeader("Authorization", "Bearer "+token)
}

// Logout resets the "Authorization" headers
func (tester *APITester) Logout() {
	tester.SetHeader("Authorization", "")
}

// CreateUser creates a new user and sets its password
func (tester *APITester) CreateUser(username, password string) {
	var info models.Userinfo
	tester.MustPostJSON("/api/v1/user", JSONObj{
		"username": username,
	}, &info)

	tester.MustPostJSON("/api/v1/user/"+username+"/password", JSONObj{
		"password": password,
	}, nil)
}

// SetT switch the current t and returns a func that restore the initial one
func (tester *APITester) SetT(t *testing.T) func() {
	t.Helper()

	save := tester.t
	tester.t = t
	restDB := tester.DB.SetTB(t)
	return func() {
		restDB()
		tester.t = save
	}
}

// Run runs the test as t.Run would, and call SetTB in addition
func (tester *APITester) Run(name string, run func(t *testing.T)) bool {
	tester.t.Helper()

	return tester.t.Run(name, func(t *testing.T) {
		defer tester.TestLogger.SetTB(t)()
		defer tester.SetT(t)()
		run(t)
	})
}

// LoadFixture loads a fixture set into the database
func (tester *APITester) LoadFixture(data io.Reader) {
	tester.fixtureLoader.Load(tester.t, data)
}

// LoadFixtureJSON loads a json-encoded fixture
func (tester *APITester) LoadFixtureJSON(data []byte) {
	tester.fixtureLoader.LoadJSON(tester.t, data)
}

// LoadFixtureFile loads a json-encoded fixture file
func (tester *APITester) LoadFixtureFile(filename string) {
	tester.fixtureLoader.LoadFile(tester.t, filename)
}
