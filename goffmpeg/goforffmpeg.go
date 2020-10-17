// goforffmpeg

package main

import (
	"bytes"
	"context"
	"flag"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/micro/go-config"
	"github.com/micro/go-config/source/file"
	"github.com/minio/minio-go"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
	"gopkg.in/avro.v0"
)

type Worker struct {
	Message_Params *AvroParams
	Output         chan string
}

type KReqMessage struct {
	// Message map[string]map[string]string `avro:Message`
	Job_id string      `avro:jobid`
	Params *AvroParams `avro:params`
}

type AvroParams struct {
	Command            string
	Parameters         string
	Endpoint           string
	Access_key_id      string
	Secret_access_key  string
	Bucket_name        string
	Region             string
	Upload_file_name   string
	Upload_file_path   string
	Download_file_name string
	Download_file_path string
	Content_type       string
}

type KResMessage struct {
	Id      string `avro:"id"`
	Message string `avro:"message"`
}

func (cmd *Worker) Run() {
	minioClient, err := minio.New(cmd.Message_Params.Endpoint, cmd.Message_Params.Access_key_id, cmd.Message_Params.Secret_access_key, false)
	if err != nil {
		log.Error().Msgf("Minio client error: %s\n", err.Error())
	}
	err = minioClient.MakeBucket(cmd.Message_Params.Bucket_name, cmd.Message_Params.Region)
	if err != nil {
		// Check to see if we already own this bucket (which happens if you run this twice)
		exists, err := minioClient.BucketExists(cmd.Message_Params.Bucket_name)
		if err == nil && exists {
			log.Info().Msgf("We already own %s\n", cmd.Message_Params.Bucket_name)
		} else {
			log.Error().Msgf("Bucket control error: %s\n", err.Error())
		}
	} else {
		log.Info().Msgf("Successfully created %s\n", cmd.Message_Params.Bucket_name)
	}
	err = minioClient.FGetObject(cmd.Message_Params.Bucket_name, cmd.Message_Params.Download_file_name, cmd.Message_Params.Download_file_path, minio.GetObjectOptions{})
	if err != nil {
		log.Error().Msgf("Object get error: %s\n", err.Error())
	} else {
		log.Info().Msgf("Object %s downloaded successfully \n", cmd.Message_Params.Download_file_name)
	}
	parameters := strings.Split(cmd.Message_Params.Parameters, " ")
	out, err := exec.Command(cmd.Message_Params.Command, parameters...).Output()
	if err != nil {
		log.Error().Msgf("Command execution error: %s\n", err.Error())
	}
	putinfo, err := minioClient.FPutObject(cmd.Message_Params.Bucket_name, cmd.Message_Params.Upload_file_name, cmd.Message_Params.Upload_file_path, minio.PutObjectOptions{ContentType: cmd.Message_Params.Content_type})
	log.Info().Msgf("File upload info: %d bytes uploaded", putinfo)
	if err != nil {
		log.Error().Msgf("Error on uploading file: %s\n", err.Error())
	}

	cmd.Output <- string(out)
}

func Collect(c chan string, w *kafka.Writer, aw *avro.SpecificDatumWriter) {
	for {
		msg := <-c
		message := &KResMessage{Id: "TESTID", Message: msg}
		buffer := bytes.Buffer{}
		encoder := avro.NewBinaryEncoder(&buffer)
		err := aw.Write(message, encoder)
		if err != nil {
			log.Error().Msgf("Avro message encoding error: %s\n", err.Error())
		}
		err = w.WriteMessages(context.Background(),
			kafka.Message{
				Key:   []byte("TEST"),
				Value: buffer.Bytes(),
			},
		)
		if err != nil {
			log.Error().Msgf("Kafka send message error: : %s\n", err.Error())
			break
		}

	}
}

func main() {
	runtime.GOMAXPROCS(4)
	configlocation := flag.String("c", "c", "config location")
	flag.Parse()
	config.Load(file.NewSource(
		file.WithPath(*configlocation),
	))
	reqschema, err := avro.ParseSchemaFile("schema.avsc")
	if err != nil {
		log.Error().Msgf("Request schema parse error: %s\n", err.Error())
	}
	respschema, err := avro.ParseSchemaFile("respschema.avsc")
	if err != nil {
		log.Error().Msgf("Response schema parse error: %s\n", err.Error())
	}
	brokerurl := config.Get("kafkaBrokerUrl").StringSlice([]string{})
	ConsumerTopic := config.Get("kafkaConsumerTopic").String("kafkaConsumerTopic")
	clientId := config.Get("kafkaClientId").String("kafkaClientId")
	ProducerTopic := config.Get("kafkaProducerTopic").String("kafkaProducerTopic")

	Rconfig := kafka.ReaderConfig{
		Brokers:         brokerurl,
		Topic:           ConsumerTopic,
		GroupID:         clientId,
		MinBytes:        10e3,            // 10KB
		MaxBytes:        10e6,            // 10MB
		MaxWait:         1 * time.Second, // Maximum amount of time to wait for new data to come when fetching batches of messages from kafka.
		ReadLagInterval: -1,
	}

	Wconfig := kafka.WriterConfig{
		Brokers:  brokerurl,
		Topic:    ProducerTopic,
		Balancer: &kafka.LeastBytes{},
	}

	out := make(chan string)
	r := kafka.NewReader(Rconfig)
	w := kafka.NewWriter(Wconfig)
	areader := avro.NewSpecificDatumReader()
	areader.SetSchema(reqschema)
	writer := avro.NewSpecificDatumWriter()
	writer.SetSchema(respschema)
	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			log.Error().Msgf("Reading message from kafka error: %s\n", err.Error())
			continue
		}

		decoder := avro.NewBinaryDecoder(m.Value)
		decodedRecord := new(KReqMessage)
		err = areader.Read(decodedRecord, decoder)
		if err != nil {
			log.Error().Msgf("Decoding avro message error: %s\n", err.Error())
			continue
		}
		worker := &Worker{Message_Params: decodedRecord.Params, Output: out}
		go worker.Run()
		go Collect(out, w, writer)
	}
	r.Close()
	w.Close()
}
