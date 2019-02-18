package rajasms

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/minchao/smsender/smsender/model"
	"github.com/minchao/smsender/smsender/plugin"
	"github.com/minchao/smsender/smsender/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
)

const name = "rajasms"
const path = "/sms/api_sms_masking_send_json.php"

func init() {
	plugin.RegisterProvider(name, Plugin)
}

func Plugin(config *viper.Viper) (model.Provider, error) {
	return Config{
		Key:           config.GetString("key"),
		Server:        config.GetString("server"),
		WebhookServer: config.GetString("webhook.server"),
		EnableWebhook: config.GetBool("webhook.enable"),
	}.New(name)
}

type Provider struct {
	name          string
	server        string
	enableWebhook bool
	webhookPath   string
	APIKey        string
	WebhookServer string
}

type Config struct {
	Key           string
	Secret        string
	Server        string
	WebhookServer string
	EnableWebhook bool
}

type RajaRequest struct {
	APIkey      string        `json:"apikey"`
	CallbackURL string        `json:"callbackurl"`
	DataPacket  []RajaMessage `json:"datapacket"`
}

type RajaMessage struct {
	Number      string `json:"number"`
	Message     string `json:"message"`
	SendingTime string `json:"sendingdatetime"`
}

type RajaResponse struct {
	SendingResponse []SendingResponse `json:"sending_respon"`
}

type SendingResponse struct {
	GlobalStatus     int              `json:"globalstatus"`
	GlobalStatusText string           `json:"globalstatustext"`
	DataPacket       []ResponsePacket `json:"datapacket"`
}

type ResponsePacket struct {
	Packet PacketObject `json:"packet"`
}

type PacketObject struct {
	Number            string `json:"number"`
	SendingID         uint64 `json:"sendingid"`
	SendingStatus     int    `json:"sendingstatus"`
	SendingStatusText string `json:"sendingstatustext"`
	Price             int    `json:"price"`
}

type CallbackResponse struct {
	SendingReponse []DeliveryResponse `json:"status_respon"`
}

type DeliveryResponse struct {
	SendingID          uint64 `json:"sendingid"`
	Number             string `json:"number"`
	DeliveryStatus     string `json:"deliverystatus"`
	DeliveryStatusText string `json:"deliverystatustext"`
}

// New creates Nexmo Provider.
func (c Config) New(name string) (*Provider, error) {
	return &Provider{
		name:          name,
		server:        c.Server,
		enableWebhook: c.EnableWebhook,
		webhookPath:   "/webhooks/" + name,
		WebhookServer: c.WebhookServer,
		APIKey:        c.Key,
	}, nil
}

func (b Provider) Name() string {
	return b.name
}

func (b Provider) Send(message model.Message) *model.MessageResponse {
	ctx, span := trace.StartSpan(context.Background(), "Smsender.rajasms.send")
	defer span.End()

	cb := ""

	if b.enableWebhook {
		cb = b.WebhookServer + b.webhookPath
	}

	req := RajaRequest{
		APIkey:      b.APIKey,
		CallbackURL: cb,
		DataPacket: []RajaMessage{
			{
				Number:  message.To,
				Message: message.Body,
			},
		},
	}
	o, err := json.Marshal(req)
	if err != nil {
		log.Error(err)
		return nil
	}
	bp := bytes.NewBuffer(o)
	url := b.server + path
	//log.Debug("SMS Message ", string(o))
	log.Info("URL ", url)
	log.Info("Payload ", string(o))
	client := &http.Client{Transport: &ochttp.Transport{}}

	r, _ := http.NewRequest("POST", url, bp)

	// Propagate the trace header info in the outgoing requests.
	r = r.WithContext(ctx)
	resp, err := client.Do(r)
	if err != nil {
		log.Error("Fail calling RajaSMS")
		log.Println(err)
		log.Error("Response status code", resp.StatusCode)
		log.Error("Response status", resp.Status)
		log.Error("Response body ", resp.Body)
		return nil
		//return err
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	log.Info("response ", string(data))
	var obj RajaResponse
	if err := json.Unmarshal(data, &obj); err != nil {
		log.Error("Error unmarshall ", err)
		return nil
	}

	log.Info(obj)
	var status model.StatusCode
	var providerMessageID string
	var rsObj interface{}
	if len(obj.SendingResponse) > 0 && len(obj.SendingResponse[0].DataPacket) > 0 {
		pkg := obj.SendingResponse[0].DataPacket[0].Packet
		status = convertStatus(strconv.Itoa(pkg.SendingStatus))
		providerMessageID = strconv.Itoa(int(pkg.SendingID))
		rsObj = pkg
	} else {
		rsObj = nil
		status = model.StatusFailed
	}

	return model.NewMessageResponse(status, rsObj, &providerMessageID)
}

// Callback see https://docs.nexmo.com/messaging/sms-api/api-reference#delivery_receipt
func (b Provider) Callback(register func(webhook *model.Webhook), receiptsCh chan<- model.MessageReceipt) {
	if !b.enableWebhook {
		return
	}

	register(&model.Webhook{
		Path: b.webhookPath,
		Func: func(w http.ResponseWriter, r *http.Request) {
			var receipt CallbackResponse
			err := utils.GetInput(r.Body, &receipt, nil)
			if err != nil {
				log.Errorf("webhooks '%s' json unmarshal error: %+v", b.name, receipt)

				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if len(receipt.SendingReponse) == 0 || receipt.SendingReponse[0].SendingID == 0 || receipt.SendingReponse[0].DeliveryStatus == "" {
				log.Infof("webhooks '%s' empty request body", b.name)

				// When you set the callback URL for delivery receipt,
				// Nexmo will send several requests to make sure that webhook was okay (status code 200).
				w.WriteHeader(http.StatusOK)
				return
			}

			receiptsCh <- *model.NewMessageReceipt(
				strconv.Itoa(int(receipt.SendingReponse[0].SendingID)),
				b.Name(),
				convertDeliveryReceiptStatus(receipt.SendingReponse[0].DeliveryStatus),
				receipt)

			w.WriteHeader(http.StatusOK)
		},
		Method: "POST",
	})
}

func convertStatus(rawStatus string) model.StatusCode {
	var status model.StatusCode
	switch rawStatus {
	case "10":
		status = model.StatusSent
	default:
		status = model.StatusFailed
	}
	return status
}

func convertDeliveryReceiptStatus(rawStatus string) model.StatusCode {
	var status model.StatusCode
	switch rawStatus {
	case "1", "2":
		status = model.StatusSent
	case "3":
		status = model.StatusDelivered
	case "4", "7":
		status = model.StatusUndelivered
	default:
		// expired, unknown
		status = model.StatusUnknown
	}
	return status
}
