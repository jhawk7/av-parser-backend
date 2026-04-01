package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jhawk7/av-parser-service/internal/common"
	"github.com/jhawk7/av-parser-service/internal/mqttclient"
	"github.com/redis/go-redis/v9"
)

type StorageClient struct {
	redisClient *redis.Client
}

func InitStorageClient(config *common.Config) *StorageClient {
	redisCLient := redis.NewClient(&redis.Options{
		Addr:     config.RedisHost,
		Password: config.RedisPass,
		DB:       0,
	})

	res, connErr := redisCLient.Ping(context.TODO()).Result()
	if connErr != nil {
		err := fmt.Errorf("failed to connect to redis server; %v", connErr)
		common.LogError(err, true)
	}

	common.LogInfo(fmt.Sprintf("successfully connected to redis server; ping response: %v", res))

	return &StorageClient{
		redisClient: redisCLient,
	}
}

func (s *StorageClient) GetAllJobs(ctx context.Context) []mqttclient.AVMsg {
	keys, err := s.redisClient.Keys(ctx, "*").Result()
	if err != nil {
		err := fmt.Errorf("failed to retrieve keys from redis; %v", err)
		common.LogError(err, false)
		return nil
	}

	var jobs []mqttclient.AVMsg
	for _, key := range keys {
		val, getErr := s.redisClient.Get(ctx, key).Result()
		if getErr != nil {
			err := fmt.Errorf("failed to retrieve value for key %v from redis; %v", key, getErr)
			common.LogError(err, false)
			continue
		}

		var avMsg mqttclient.AVMsg
		jsonErr := json.Unmarshal([]byte(val), &avMsg)
		if jsonErr != nil {
			err := fmt.Errorf("failed to unmarshal value for key %v from redis; %v", key, jsonErr)
			common.LogError(err, false)
			continue
		}

		jobs = append(jobs, avMsg)
	}

	return jobs
}

func (s *StorageClient) StoreRequest(avMsg *mqttclient.AVMsg) {
	guuid := s.generateUUID()
	avMsg.Id = guuid

	jsonBytes, jsonErr := json.Marshal(avMsg)
	if jsonErr != nil {
		err := fmt.Errorf("failed to marshal avMsg to json; %v", jsonErr)
		common.LogError(err, false)
	}

	if setErr := s.redisClient.Set(context.TODO(), guuid, jsonBytes, (time.Hour * 24 * 7)).Err(); setErr != nil {
		err := fmt.Errorf("failed to store av request %v in redis; %v", guuid, setErr)
		common.LogError(err, false)
		return
	}

	avMsg.Status = "pending"
	common.LogInfo(fmt.Sprintf("successfully stored av request with id %v in redis", guuid))
}

func (s *StorageClient) UpdateRequest(avMsg *mqttclient.AVMsg) {
	jsonBytes, jsonErr := json.Marshal(avMsg)
	if jsonErr != nil {
		err := fmt.Errorf("failed to marshal avMsg to json; %v", jsonErr)
		common.LogError(err, false)
	}

	if setErr := s.redisClient.Set(context.TODO(), avMsg.Id, jsonBytes, (time.Hour * 24 * 7)).Err(); setErr != nil {
		err := fmt.Errorf("failed to update av request %v in redis; %v", avMsg.Id, setErr)
		common.LogError(err, false)
		return
	}

	common.LogInfo(fmt.Sprintf("successfully updated av request with id %v in redis", avMsg.Id))
}

func (s *StorageClient) generateUUID() string {
	guuid := uuid.New().String()
	guuid = generateString()
	for s.redisClient.Exists(context.TODO(), guuid).Val() == 1 {
		guuid = generateString()
		common.LogInfo(fmt.Sprintf("generated key %v already exists; re-generating", guuid))
	}

	return guuid
}

func generateString() string {
	uuid := uuid.New().String()
	return strings.ReplaceAll(uuid, "-", "")[0:8]
}
