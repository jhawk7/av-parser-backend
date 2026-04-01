package mqttclient

import (
	"encoding/json"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jhawk7/av-parser-service/internal/common"
)

var (
	msgChan chan mqtt.Message
)

type IMQTTConsumer interface {
	Listen(outCh chan<- AVMsg)
	Disconnect()
}

type consumer struct {
	mqttClient mqtt.Client
	topics     []string
}

type AVMsg struct {
	Url       string `json:"url"`
	Type      string `json:"type"`
	Id        string `json:"id,omitempty"`
	Status    string `json:"status,omitempty"`
	Timestamp string `json:"timestamp"`
}

var connHandler mqtt.OnConnectHandler = func(mclient mqtt.Client) {
	common.LogInfo("successfully connected to mqtt server")
}

var lostHandler mqtt.ConnectionLostHandler = func(mclient mqtt.Client, err error) {
	connErr := fmt.Errorf("lost connection to mqtt server; %v", err)
	common.LogError(connErr, false)
}

var msgHandler mqtt.MessageHandler = func(mclient mqtt.Client, msg mqtt.Message) {
	info := fmt.Sprintf("messaged received [id: %d] [topic: %v], [payload: %s]", msg.MessageID(), msg.Topic(), msg.Payload())
	common.LogInfo(info)

	msgChan <- msg
}

func InitClient(config *common.Config) IMQTTConsumer {
	//set client options
	broker := config.MQTTServer
	port := config.MQTTPort
	user := config.MQTTUser
	pass := config.MQTTPass
	topics := config.MQTTTopics

	opts := mqtt.NewClientOptions().AddBroker(fmt.Sprintf("ws://%v:%v", broker, port))
	opts.SetClientID("av-parser-consumer")
	opts.SetUsername(user)
	opts.SetPassword(pass)
	opts.SetKeepAlive(time.Second * 10)
	opts.SetCleanSession(false) //disabling clean session on client reconnect so that messages will resume on reconnect
	opts.OnConnect = connHandler
	opts.OnConnectionLost = lostHandler
	mclient := mqtt.NewClient(opts)
	if token := mclient.Connect(); token.Wait() && token.Error() != nil {
		err := fmt.Errorf("mqtt connection failed; %v", token.Error())
		common.LogError(err, true)
	}

	msgChan = make(chan mqtt.Message, 1)
	c := &consumer{mqttClient: mclient, topics: topics}
	return c
}

func (c *consumer) Listen(avChan chan<- AVMsg) {
	c.subscribe()
	for msg := range msgChan {
		var avMsg AVMsg
		common.LogInfo(fmt.Sprintf("parsing message from topic %v", msg.Topic()))
		if deserializeErr := json.Unmarshal(msg.Payload(), &avMsg); deserializeErr != nil {
			err := fmt.Errorf("failed to deserialize message; %v", deserializeErr)
			common.LogError(err, false)
			continue
		}

		common.LogInfo(fmt.Sprintf("message deserialized successfully; [url: %v], [flag: %v]", avMsg.Url, avMsg.Type))
		msg.Ack() //ack message after successful deserialization
		//export parsed message for av processing
		avChan <- avMsg
	}
}

func (c *consumer) subscribe() {
	filters := make(map[string]byte)
	for _, topic := range c.topics {
		common.LogInfo(fmt.Sprintf("subscribing to topic [%v]", topic))
		filters[topic] = 2 //qos: 0 - no standard, 1 - "atleast once", 2 - exactly once
	}

	if token := c.mqttClient.SubscribeMultiple(filters, msgHandler); token.Wait() && token.Error() != nil {
		err := fmt.Errorf("failed to subscribe to topics %v; error %v", c.topics, token.Error())
		common.LogError(err, true)
	}
	common.LogInfo(fmt.Sprintf("subscribed to topics %v", c.topics))
}

func (c *consumer) Disconnect() {
	common.LogInfo("disconnecting from mqtt server..")
	c.mqttClient.Disconnect(5000)
}
