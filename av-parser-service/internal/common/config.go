package common

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

type Config struct {
	MQTTServer string
	MQTTPort   string
	MQTTUser   string
	MQTTPass   string
	MQTTTopics []string
	RedisHost  string
	RedisPass  string
}

func LoadConfig() *Config {
	var config Config
	if server, exists := os.LookupEnv("MQTT_SERVER"); exists {
		config.MQTTServer = server
	} else {
		LogError(fmt.Errorf("MQTT_SERVER environment variable not set"), true)
	}

	if port, exists := os.LookupEnv("MQTT_PORT"); exists {
		config.MQTTPort = port
	} else {
		LogError(fmt.Errorf("MQTT_PORT environment variable not set"), true)
	}

	if user, exists := os.LookupEnv("MQTT_USER"); exists {
		config.MQTTUser = user
	} else {
		LogError(fmt.Errorf("MQTT_USER environment variable not set"), true)
	}

	if pass, exists := os.LookupEnv("MQTT_PASS"); exists {
		config.MQTTPass = pass
	} else {
		LogError(fmt.Errorf("MQTT_PASS environment variable not set"), true)
	}

	if topics, exists := os.LookupEnv("MQTT_TOPICS"); exists {
		config.MQTTTopics = strings.Split(topics, ",")
	} else {
		LogError(fmt.Errorf("MQTT_TOPICS environment variable not set"), true)
	}

	if redisHost, exists := os.LookupEnv("REDIS_HOST"); exists {
		config.RedisHost = redisHost
	} else {
		LogError(fmt.Errorf("REDIS_HOST environment variable not set"), true)
	}

	if redisPass, exists := os.LookupEnv("REDIS_PASS"); exists {
		config.RedisPass = redisPass
	} else {
		LogError(fmt.Errorf("REDIS_PASS environment variable not set"), true)
	}

	return &config
}

func LogError(err error, fatal bool) {
	if err != nil {
		logrus.Errorf("error: %v", err)
		if fatal {
			panic(err)
		}
	}
}

func LogInfo(info string) {
	logrus.Info(info)
}
