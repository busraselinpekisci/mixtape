// metago

package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/micro/go-config"
	"github.com/micro/go-config/source/file"
	"github.com/minio/minio-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
	"gopkg.in/avro.v0"
)

type Worker struct {
	Message_Params *AvroParams
	Output         chan *AvroParams
}

type AvroParams struct {
	Job_id       string
	Params       *ParamStruct
	Job_response []CommandsStruct
	Sequence     *SequenceStruct
	Status       *avro.GenericEnum
	Errors       []string
	Version      string
}

type SequenceStruct struct {
	Current_job    string
	Current_job_id string
	Main_job_id    string
}

type ParamStruct struct {
	S3_info  *S3InfoStruct
	Commands []CommandsStruct
}
type CommandsStruct struct {
	Cmd_name string
	Cmd      string
	Stdout   string
	Upload   string
}
type S3InfoStruct struct {
	Source      *SourceStruct
	Destination *DestinationStruct
}

type CredentialStruct struct {
	Endpoint          string
	Access_key_id     string
	Secret_access_key string
	Region            string
}

type SourceStruct struct {
	S3_credential *CredentialStruct
	S3_file       *S3FileStruct
}

type DestinationStruct struct {
	S3_credential *CredentialStruct
	S3_file       *S3FileStruct
}

type S3FileStruct struct {
	Bucket    string
	Path      string
	Name      string
	Pure_name string
}

func log_with_delay(r *kafka.Reader, w *kafka.Writer, delay_time time.Duration) {
	for {
		rstats := r.Stats()
		wstats := w.Stats()
		log.Info().Msgf("Kafka reader error count: %v , waitTime duration: %v", rstats.Errors, rstats.WaitTime)
		log.Info().Msgf("Kafka writer error count: %v , waitTime duration: %v", wstats.Errors, wstats.WaitTime)
		time.Sleep(delay_time * time.Second)
	}
}

func configure_logging(file_path string) {
	if file_path != "" {
		file, err := os.OpenFile(file_path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
		} else {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: file, NoColor: true, TimeFormat: time.RFC3339})
		}
	}
}

func (cmd *Worker) Run() {

	minioClient, err := minio.New(cmd.Message_Params.Params.S3_info.Source.S3_credential.Endpoint, cmd.Message_Params.Params.S3_info.Source.S3_credential.Access_key_id, cmd.Message_Params.Params.S3_info.Source.S3_credential.Secret_access_key, false)

	if err != nil {
		log.Error().Msgf("Minio client error: %s\n", err.Error())
		cmd.Message_Params.Status.Set("FAILURE")
		cmd.Output <- cmd.Message_Params
		return
	}
	err = minioClient.MakeBucket(cmd.Message_Params.Params.S3_info.Source.S3_file.Bucket, "us-east-1")
	if err != nil {
		// Check to see if we already own this bucket (which happens if you run this twice)
		exists, err := minioClient.BucketExists(cmd.Message_Params.Params.S3_info.Source.S3_file.Bucket)
		if err == nil && exists {
			log.Info().Msgf("We already own %s\n", cmd.Message_Params.Params.S3_info.Source.S3_file.Bucket)
		} else {
			log.Error().Msgf("Bucket control error: %s\n", err.Error())
			cmd.Message_Params.Status.Set("FAILURE")
			cmd.Output <- cmd.Message_Params
			return
		}
	} else {
		log.Info().Msgf("Successfully created %s\n", cmd.Message_Params.Params.S3_info.Source.S3_file.Bucket)
	}
	err = minioClient.FGetObject(cmd.Message_Params.Params.S3_info.Source.S3_file.Bucket, cmd.Message_Params.Params.S3_info.Source.S3_file.Path+"/"+cmd.Message_Params.Params.S3_info.Source.S3_file.Name, "/tmp/"+cmd.Message_Params.Params.S3_info.Source.S3_file.Path+"/"+cmd.Message_Params.Params.S3_info.Source.S3_file.Name, minio.GetObjectOptions{})
	if err != nil {
		log.Error().Msgf("Object get error: %s\n", err.Error())
		cmd.Message_Params.Status.Set("FAILURE")
		cmd.Output <- cmd.Message_Params
		return
	} else {
		log.Info().Msgf("Object %v downloaded successfully \n", cmd.Message_Params.Params.S3_info.Source.S3_file.Name)
	}
	for _, c := range cmd.Message_Params.Params.Commands {
		out, err := exec.Command("sh", "-c", c.Cmd).Output()
		if err != nil {
			log.Error().Msgf("Command execution error: %s\n", err.Error())
			cmd.Message_Params.Status.Set("FAILURE")
			cmd.Output <- cmd.Message_Params
			return
		}
		if c.Stdout == "True" {
			if len(cmd.Message_Params.Job_response) != 0 {
				cmd.Message_Params.Job_response[len(cmd.Message_Params.Job_response)-1] = c
			} else {
				cmd.Message_Params.Job_response = append(cmd.Message_Params.Job_response, c)

			}
			cmd.Message_Params.Job_response[len(cmd.Message_Params.Job_response)-1].Stdout = string(out)
		}

	}
	cmd.Message_Params.Status.Set("SUCCESS")
	log.Info().Msgf("Job-[%s] is finished successfully", cmd.Message_Params.Job_id)
	cmd.Output <- cmd.Message_Params
}

func Collect(c chan *AvroParams, w *kafka.Writer, aw *avro.SpecificDatumWriter) {
	for {
		msg := <-c
		message := msg
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
	log.Info().Msgf("Go-FFmpeg-Meta has started")
	runtime.GOMAXPROCS(4)
	configlocation := flag.String("c", "c", "config location")
	log_file_path := flag.String("l", "", "log file path")
	flag.Parse()
	config.Load(file.NewSource(
		file.WithPath(*configlocation),
	))
	configure_logging(*log_file_path)
	reqschema, err := avro.ParseSchemaFile(config.Get("requestSchemaPath").String("requestSchemaPath"))
	if err != nil {
		log.Error().Msgf("Request schema parse error: %s\n", err.Error())
	}
	respschema, err := avro.ParseSchemaFile(config.Get("responseSchemaPath").String("responseSchemaPath"))
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
	// Kafka startup control
	for err == nil {
		conn, err := kafka.Dial("tcp", brokerurl[0])
		if err != nil {
			log.Error().Msgf("Kafka connection error: %s\n", err.Error())
			time.Sleep(5 * time.Second)
		} else {
			err = conn.Close()
			if err == nil {
				log.Info().Msgf("Connection successful on Kafka at %v", brokerurl[0])
				break
			} else {
				continue
			}
		}
	}

	out := make(chan *AvroParams)
	r := kafka.NewReader(Rconfig)
	w := kafka.NewWriter(Wconfig)
	areader := avro.NewSpecificDatumReader()
	areader.SetSchema(reqschema)
	writer := avro.NewSpecificDatumWriter()
	writer.SetSchema(respschema)
	go log_with_delay(r, w, 10)
	for {
		m, err := r.ReadMessage(context.Background())

		if err != nil {
			log.Error().Msgf("Reading message from kafka error: %s\n", err.Error())
			continue
		}

		decoder := avro.NewBinaryDecoder(m.Value)
		decodedRecord := new(AvroParams)
		err = areader.Read(decodedRecord, decoder)
		if err != nil {
			log.Error().Msgf("Decoding avro message error: %s\n", err.Error())
			continue
		}
		worker := &Worker{Message_Params: decodedRecord, Output: out}
		go worker.Run()
		go Collect(out, w, writer)
	}
	r.Close()
	w.Close()
}
