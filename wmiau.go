package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/jmoiron/sqlx"
	"github.com/mdp/qrterminal/v3"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type MyClient struct {
	WAClient       *whatsmeow.Client
	eventHandlerID uint32
	userID         string
	token          string
	subscriptions  []string
	db             *sqlx.DB
}

func sendToGlobalWebHook(jsonData []byte, token string, userID string) {
	jsonDataStr := string(jsonData)

	instance_name := ""
	userinfo, found := userinfocache.Get(token)
	if found {
		instance_name = userinfo.(Values).Get("Name")
	}

	if *globalWebhook != "" {
		log.Info().Str("url", *globalWebhook).Msg("Calling global webhook")
		globalData := map[string]string{
			"jsonData":     jsonDataStr,
			"token":        token,
			"userID":       userID,
			"instanceName": instance_name,
		}
		callHook(*globalWebhook, globalData, userID)
	}
}

func sendToUserWebHook(webhookurl string, path string, jsonData []byte, userID string, token string) {
	instance_name := ""
	userinfo, found := userinfocache.Get(token)
	if found {
		instance_name = userinfo.(Values).Get("Name")
	}
	data := map[string]string{
		"jsonData":     string(jsonData),
		"token":        token,
		"instanceName": instance_name,
	}

	log.Debug().Interface("webhookData", data).Msg("Data being sent to webhook")

	if webhookurl != "" {
		log.Info().Str("url", webhookurl).Msg("Calling user webhook")
		if path == "" {
			go callHook(webhookurl, data, userID)
		} else {
			errChan := make(chan error, 1)
			go func() {
				err := callHookFile(webhookurl, data, userID, path)
				errChan <- err
			}()
			if err := <-errChan; err != nil {
				log.Error().Err(err).Msg("Error calling hook file")
			}
		}
	} else {
		log.Warn().Str("userid", userID).Msg("No webhook set for user")
	}
}

func updateAndGetUserSubscriptions(mycli *MyClient) ([]string, error) {
	currentEvents := ""
	userinfo2, found2 := userinfocache.Get(mycli.token)
	if found2 {
		currentEvents = userinfo2.(Values).Get("Events")
	} else {
		if err := mycli.db.Get(&currentEvents, "SELECT events FROM users WHERE id=$1", mycli.userID); err != nil {
			log.Warn().Err(err).Str("userID", mycli.userID).Msg("Could not get events from DB")
			return nil, err
		}
	}

	eventarray := strings.Split(currentEvents, ",")
	var subscribedEvents []string
	if len(eventarray) == 1 && eventarray[0] == "" {
		subscribedEvents = []string{}
	} else {
		for _, arg := range eventarray {
			arg = strings.TrimSpace(arg)
			if arg != "" && Find(supportedEventTypes, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}
		}
	}

	mycli.subscriptions = subscribedEvents
	return subscribedEvents, nil
}

func getUserWebhookUrl(token string) string {
	webhookurl := ""
	myuserinfo, found := userinfocache.Get(token)
	if !found {
		log.Warn().Str("token", token).Msg("Could not call webhook as there is no user for this token")
	} else {
		webhookurl = myuserinfo.(Values).Get("Webhook")
	}
	return webhookurl
}

func sendEventWithWebHook(mycli *MyClient, postmap map[string]interface{}, path string) {
	webhookurl := getUserWebhookUrl(mycli.token)

	subscribedEvents, err := updateAndGetUserSubscriptions(mycli)
	if err != nil {
		return
	}

	eventType, ok := postmap["type"].(string)
	if !ok {
		log.Error().Msg("Event type is not a string in postmap")
		return
	}

	log.Debug().
		Str("userID", mycli.userID).
		Str("eventType", eventType).
		Strs("subscribedEvents", subscribedEvents).
		Msg("Checking event subscription")

	checkIfSubscribedInEvent := checkIfSubscribedToEvent(subscribedEvents, postmap["type"].(string), mycli.userID)
	if !checkIfSubscribedInEvent {
		return
	}

	jsonData, err := json.Marshal(postmap)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal postmap to JSON")
		return
	}

	sendToUserWebHook(webhookurl, path, jsonData, mycli.userID, mycli.token)
	go sendToGlobalWebHook(jsonData, mycli.token, mycli.userID)
	go sendToGlobalRabbit(jsonData)
}

func checkIfSubscribedToEvent(subscribedEvents []string, eventType string, userId string) bool {
	if !Find(subscribedEvents, eventType) && !Find(subscribedEvents, "All") {
		log.Warn().
			Str("type", eventType).
			Strs("subscribedEvents", subscribedEvents).
			Str("userID", userId).
			Msg("Skipping webhook. Not subscribed for this type")
		return false
	}
	return true
}

func (s *server) connectOnStartup() {
	rows, err := s.db.Queryx("SELECT id,name,token,jid,webhook,events,proxy_url,CASE WHEN s3_enabled THEN 'true' ELSE 'false' END AS s3_enabled,media_delivery FROM users WHERE connected=1")
	if err != nil {
		log.Error().Err(err).Msg("DB Problem")
		return
	}
	defer rows.Close()
	for rows.Next() {
		txtid := ""
		token := ""
		jid := ""
		name := ""
		webhook := ""
		events := ""
		proxy_url := ""
		s3_enabled := ""
		media_delivery := ""
		err = rows.Scan(&txtid, &name, &token, &jid, &webhook, &events, &proxy_url, &s3_enabled, &media_delivery)
		if err != nil {
			log.Error().Err(err).Msg("DB Problem")
			return
		} else {
			log.Info().Str("token", token).Msg("Connect to Whatsapp on startup")
			v := Values{map[string]string{
				"Id":            txtid,
				"Name":          name,
				"Jid":           jid,
				"Webhook":       webhook,
				"Token":         token,
				"Proxy":         proxy_url,
				"Events":        events,
				"S3Enabled":     s3_enabled,
				"MediaDelivery": media_delivery,
			}}
			userinfocache.Set(token, v, cache.NoExpiration)

			eventarray := strings.Split(events, ",")

			var subscribedEvents []string
			if len(eventarray) == 1 && eventarray[0] == "" {
				subscribedEvents = []string{}
			} else {
				for _, arg := range eventarray {
					if !Find(supportedEventTypes, arg) {
						log.Warn().Str("Type", arg).Msg("Event type discarded")
						continue
					}
					if !Find(subscribedEvents, arg) {
						subscribedEvents = append(subscribedEvents, arg)
					}
				}

			}
			eventstring := strings.Join(subscribedEvents, ",")
			log.Info().Str("events", eventstring).Str("jid", jid).Msg("Attempt to connect")
			killchannel[txtid] = make(chan bool)
			go s.startClient(txtid, jid, token, subscribedEvents)

			go func(userID string) {
				var s3Config struct {
					Enabled       bool   `db:"s3_enabled"`
					Endpoint      string `db:"s3_endpoint"`
					Region        string `db:"s3_region"`
					Bucket        string `db:"s3_bucket"`
					AccessKey     string `db:"s3_access_key"`
					SecretKey     string `db:"s3_secret_key"`
					PathStyle     bool   `db:"s3_path_style"`
					PublicURL     string `db:"s3_public_url"`
					RetentionDays int    `db:"s3_retention_days"`
				}

				err := s.db.Get(&s3Config, `
					SELECT s3_enabled, s3_endpoint, s3_region, s3_bucket, 
						   s3_access_key, s3_secret_key, s3_path_style, 
						   s3_public_url, s3_retention_days
					FROM users WHERE id = $1`, userID)

				if err != nil {
					log.Error().Err(err).Str("userID", userID).Msg("Failed to get S3 config")
					return
				}

				if s3Config.Enabled {
					config := &S3Config{
						Enabled:       s3Config.Enabled,
						Endpoint:      s3Config.Endpoint,
						Region:        s3Config.Region,
						Bucket:        s3Config.Bucket,
						AccessKey:     s3Config.AccessKey,
						SecretKey:     s3Config.SecretKey,
						PathStyle:     s3Config.PathStyle,
						PublicURL:     s3Config.PublicURL,
						RetentionDays: s3Config.RetentionDays,
					}

					err = GetS3Manager().InitializeS3Client(userID, config)
					if err != nil {
						log.Error().Err(err).Str("userID", userID).Msg("Failed to initialize S3 client on startup")
					} else {
						log.Info().Str("userID", userID).Msg("S3 client initialized on startup")
					}
				}
			}(txtid)
		}
	}
	err = rows.Err()
	if err != nil {
		log.Error().Err(err).Msg("DB Problem")
	}
}

func parseJID(arg string) (types.JID, bool) {
	if arg[0] == '+' {
		arg = arg[1:]
	}
	if !strings.ContainsRune(arg, '@') {
		return types.NewJID(arg, types.DefaultUserServer), true
	} else {
		recipient, err := types.ParseJID(arg)
		if err != nil {
			log.Error().Err(err).Msg("Invalid JID")
			return recipient, false
		} else if recipient.User == "" {
			log.Error().Err(err).Msg("Invalid JID no server specified")
			return recipient, false
		}
		return recipient, true
	}
}

func (s *server) startClient(userID string, textjid string, token string, subscriptions []string) {
	log.Info().Str("userid", userID).Str("jid", textjid).Msg("Starting websocket connection to Whatsapp")

	var deviceStore *store.Device
	var err error

	if textjid != "" {
		jid, _ := parseJID(textjid)
		deviceStore, err = container.GetDevice(context.Background(), jid)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get device")
			deviceStore = container.NewDevice()
		}
	} else {
		log.Warn().Msg("No jid found. Creating new device")
		deviceStore = container.NewDevice()
	}

	if deviceStore == nil {
		log.Warn().Msg("No store found. Creating new one")
		deviceStore = container.NewDevice()
	}

	clientLog := waLog.Stdout("Client", *waDebug, *colorOutput)

	var client *whatsmeow.Client
	if *waDebug != "" {
		client = whatsmeow.NewClient(deviceStore, clientLog)
	} else {
		client = whatsmeow.NewClient(deviceStore, nil)
	}

	clientManager.SetWhatsmeowClient(userID, client)
	if textjid != "" {
		jid, _ := parseJID(textjid)
		deviceStore, err = container.GetDevice(context.Background(), jid)
		if err != nil {
			panic(err)
		}
	} else {
		log.Warn().Msg("No jid found. Creating new device")
		deviceStore = container.NewDevice()
	}

	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_UNKNOWN.Enum()
	store.DeviceProps.Os = osName

	clientManager.SetWhatsmeowClient(userID, client)
	mycli := MyClient{client, 1, userID, token, subscriptions, s.db}
	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)

	clientManager.SetMyClient(userID, &mycli)

	httpClient := resty.New()
	httpClient.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
	if *waDebug == "DEBUG" {
		httpClient.SetDebug(true)
	}
	httpClient.SetTimeout(30 * time.Second)
	httpClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	httpClient.OnError(func(req *resty.Request, err error) {
		if v, ok := err.(*resty.ResponseError); ok {
			log.Debug().Str("response", v.Response.String()).Msg("resty error")
			log.Error().Err(v.Err).Msg("resty error")
		}
	})

	var proxyURL string
	err = s.db.Get(&proxyURL, "SELECT proxy_url FROM users WHERE id=$1", userID)
	if err == nil && proxyURL != "" {
		httpClient.SetProxy(proxyURL)
	}
	clientManager.SetHTTPClient(userID, httpClient)

	if client.Store.ID == nil {
		qrChan, err := client.GetQRChannel(context.Background())
		if err != nil {
			if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				log.Error().Err(err).Msg("Failed to get QR channel")
				return
			}
		} else {
			err = client.Connect()
			if err != nil {
				log.Error().Err(err).Msg("Failed to connect client")
				return
			}

			myuserinfo, found := userinfocache.Get(token)

			for evt := range qrChan {
				if evt.Event == "code" {
					if *logType != "json" {
						qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
						fmt.Println("QR code:\n", evt.Code)
					}
					image, _ := qrcode.Encode(evt.Code, qrcode.Medium, 256)
					base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
					sqlStmt := `UPDATE users SET qrcode=$1 WHERE id=$2`
					_, err := s.db.Exec(sqlStmt, base64qrcode, userID)
					if err != nil {
						log.Error().Err(err).Msg(sqlStmt)
					} else {
						if found {
							v := updateUserInfo(myuserinfo, "Qrcode", base64qrcode)
							userinfocache.Set(token, v, cache.NoExpiration)
							log.Info().Str("qrcode", base64qrcode).Msg("update cache userinfo with qr code")
						}
					}

					postmap := make(map[string]interface{})
					postmap["event"] = evt.Event
					postmap["qrCodeBase64"] = base64qrcode
					postmap["type"] = "QR"

					sendEventWithWebHook(&mycli, postmap, "")

				} else if evt.Event == "timeout" {
					sqlStmt := `UPDATE users SET qrcode='' WHERE id=$1`
					_, err := s.db.Exec(sqlStmt, userID)
					if err != nil {
						log.Error().Err(err).Msg(sqlStmt)
					} else {
						if found {
							v := updateUserInfo(myuserinfo, "Qrcode", "")
							userinfocache.Set(token, v, cache.NoExpiration)
						}
					}
					log.Warn().Msg("QR timeout killing channel")
					clientManager.DeleteWhatsmeowClient(userID)
					clientManager.DeleteMyClient(userID)
					clientManager.DeleteHTTPClient(userID)
					killchannel[userID] <- true
				} else if evt.Event == "success" {
					log.Info().Msg("QR pairing ok!")
					sqlStmt := `UPDATE users SET qrcode='', connected=1 WHERE id=$1`
					_, err := s.db.Exec(sqlStmt, userID)
					if err != nil {
						log.Error().Err(err).Msg(sqlStmt)
					} else {
						if found {
							v := updateUserInfo(myuserinfo, "Qrcode", "")
							userinfocache.Set(token, v, cache.NoExpiration)
						}
					}
				} else {
					log.Info().Str("event", evt.Event).Msg("Login event")
				}
			}
		}

	} else {
		log.Info().Msg("Already logged in, just connect")
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	for {
		select {
		case <-killchannel[userID]:
			log.Info().Str("userid", userID).Msg("Received kill signal")
			client.Disconnect()
			clientManager.DeleteWhatsmeowClient(userID)
			clientManager.DeleteMyClient(userID)
			clientManager.DeleteHTTPClient(userID)
			sqlStmt := `UPDATE users SET qrcode='', connected=0 WHERE id=$1`
			_, err := s.db.Exec(sqlStmt, "", userID)
			if err != nil {
				log.Error().Err(err).Msg(sqlStmt)
			}
			return
		default:
			time.Sleep(1000 * time.Millisecond)
		}
	}
}

func fileToBase64(filepath string) (string, string, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return "", "", err
	}
	mimeType := http.DetectContentType(data)
	return base64.StdEncoding.EncodeToString(data), mimeType, nil
}

func (mycli *MyClient) myEventHandler(rawEvt interface{}) {
	txtid := mycli.userID
	postmap := make(map[string]interface{})
	postmap["event"] = rawEvt
	dowebhook := 0
	path := ""

	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		if len(mycli.WAClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := mycli.WAClient.SendPresence(types.PresenceAvailable)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to send available presence")
			} else {
				log.Info().Msg("Marked self as available")
			}
		}
	case *events.Connected, *events.PushNameSetting:
		postmap["type"] = "Connected"
		dowebhook = 1
		if len(mycli.WAClient.Store.PushName) == 0 {
			break
		}
		err := mycli.WAClient.SendPresence(types.PresenceAvailable)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to send available presence")
		} else {
			log.Info().Msg("Marked self as available")
		}
		sqlStmt := `UPDATE users SET connected=1 WHERE id=$1`
		_, err = mycli.db.Exec(sqlStmt, mycli.userID)
		if err != nil {
			log.Error().Err(err).Msg(sqlStmt)
			return
		}
	case *events.PairSuccess:
		log.Info().Str("userid", mycli.userID).Str("token", mycli.token).Str("ID", evt.ID.String()).Str("BusinessName", evt.BusinessName).Str("Platform", evt.Platform).Msg("QR Pair Success")
		jid := evt.ID
		sqlStmt := `UPDATE users SET jid=$1 WHERE id=$2`
		_, err := mycli.db.Exec(sqlStmt, jid, mycli.userID)
		if err != nil {
			log.Error().Err(err).Msg(sqlStmt)
			return
		}

		myuserinfo, found := userinfocache.Get(mycli.token)
		if !found {
			log.Warn().Msg("No user info cached on pairing?")
		} else {
			txtid = myuserinfo.(Values).Get("Id")
			token := myuserinfo.(Values).Get("Token")
			v := updateUserInfo(myuserinfo, "Jid", fmt.Sprintf("%s", jid))
			userinfocache.Set(token, v, cache.NoExpiration)
			log.Info().Str("jid", jid.String()).Str("userid", txtid).Str("token", token).Msg("User information set")
		}
	case *events.StreamReplaced:
		log.Info().Msg("Received StreamReplaced event")
		return
	case *events.Message:
		var s3Config struct {
			Enabled       string `db:"s3_enabled"`
			MediaDelivery string `db:"media_delivery"`
		}
		if err := mycli.db.Get(&s3Config, `
			SELECT 
				CASE WHEN s3_enabled THEN 'true' ELSE 'false' END AS s3_enabled,
				COALESCE(media_delivery, 'base64') AS media_delivery
			FROM users
			WHERE id = $1
		`, txtid); err != nil {
			log.Error().Err(err).Msg("onMessage: Failed to get S3 config from DB")
			s3Config.Enabled = "false"
			s3Config.MediaDelivery = "base64"
		}

		lastMessageCache.Set(mycli.userID, &evt.Info, cache.DefaultExpiration)

		postmap["type"] = "Message"
		dowebhook = 1
		metaParts := []string{fmt.Sprintf("pushname: %s", evt.Info.PushName), fmt.Sprintf("timestamp: %s", evt.Info.Timestamp)}
		if evt.Info.Type != "" {
			metaParts = append(metaParts, fmt.Sprintf("type: %s", evt.Info.Type))
		}
		if evt.Info.Category != "" {
			metaParts = append(metaParts, fmt.Sprintf("category: %s", evt.Info.Category))
		}
		if evt.IsViewOnce {
			metaParts = append(metaParts, "view once")
		}
		if evt.IsViewOnce {
			metaParts = append(metaParts, "ephemeral")
		}

		log.Info().Str("id", evt.Info.ID).Str("source", evt.Info.SourceString()).Str("parts", strings.Join(metaParts, ", ")).Msg("Message Received")

		if !*skipMedia {
			img := evt.Message.GetImageMessage()
			if img != nil {
				tmpDirectory := filepath.Join("/tmp", "user_"+txtid)
				errDir := os.MkdirAll(tmpDirectory, 0751)
				if errDir != nil {
					log.Error().Err(errDir).Msg("Could not create temporary directory")
					return
				}
				data, err := mycli.WAClient.Download(context.Background(), img)
				if err != nil {
					log.Error().Err(err).Msg("Failed to download image")
					return
				}
				exts, _ := mime.ExtensionsByType(img.GetMimetype())
				tmpPath := filepath.Join(tmpDirectory, evt.Info.ID+exts[0])
				err = os.WriteFile(tmpPath, data, 0600)
				if err != nil {
					log.Error().Err(err).Msg("Failed to save image to temporary file")
					return
				}
				if s3Config.Enabled == "true" && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
					isIncoming := evt.Info.IsFromMe == false
					contactJID := evt.Info.Sender.String()
					if evt.Info.IsGroup {
						contactJID = evt.Info.Chat.String()
					}
					s3Data, err := GetS3Manager().ProcessMediaForS3(
						context.Background(),
						txtid,
						contactJID,
						evt.Info.ID,
						data,
						img.GetMimetype(),
						filepath.Base(tmpPath),
						isIncoming,
					)
					if err != nil {
						log.Error().Err(err).Msg("Failed to upload image to S3")
					} else {
						postmap["s3"] = s3Data
					}
				}
				if s3Config.MediaDelivery == "base64" || s3Config.MediaDelivery == "both" {
					base64String, mimeType, err := fileToBase64(tmpPath)
					if err != nil {
						log.Error().Err(err).Msg("Failed to convert image to base64")
						return
					}
					postmap["base64"] = base64String
					postmap["mimeType"] = mimeType
					postmap["fileName"] = filepath.Base(tmpPath)
				}
				err = os.Remove(tmpPath)
				if err != nil {
					log.Error().Err(err).Msg("Failed to delete temporary file")
				}
			}

			audio := evt.Message.GetAudioMessage()
			if audio != nil {
				tmpDirectory := filepath.Join("/tmp", "user_"+txtid)
				errDir := os.MkdirAll(tmpDirectory, 0751)
				if errDir != nil {
					log.Error().Err(errDir).Msg("Could not create temporary directory")
					return
				}
				data, err := mycli.WAClient.Download(context.Background(), audio)
				if err != nil {
					log.Error().Err(err).Msg("Failed to download audio")
					return
				}
				exts, _ := mime.ExtensionsByType(audio.GetMimetype())
				var ext string
				if len(exts) > 0 {
					ext = exts[0]
				} else {
					ext = ".ogg"
				}
				tmpPath := filepath.Join(tmpDirectory, evt.Info.ID+ext)
				err = os.WriteFile(tmpPath, data, 0600)
				if err != nil {
					log.Error().Err(err).Msg("Failed to save audio to temporary file")
					return
				}
				if s3Config.Enabled == "true" && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
					isIncoming := evt.Info.IsFromMe == false
					contactJID := evt.Info.Sender.String()
					if evt.Info.IsGroup {
						contactJID = evt.Info.Chat.String()
					}
					s3Data, err := GetS3Manager().ProcessMediaForS3(
						context.Background(),
						txtid,
						contactJID,
						evt.Info.ID,
						data,
						audio.GetMimetype(),
						filepath.Base(tmpPath),
						isIncoming,
					)
					if err != nil {
						log.Error().Err(err).Msg("Failed to upload audio to S3")
					} else {
						postmap["s3"] = s3Data
					}
				}
				if s3Config.MediaDelivery == "base64" || s3Config.MediaDelivery == "both" {
					base64String, mimeType, err := fileToBase64(tmpPath)
					if err != nil {
						log.Error().Err(err).Msg("Failed to convert audio to base64")
						return
					}
					postmap["base64"] = base64String
					postmap["mimeType"] = mimeType
					postmap["fileName"] = filepath.Base(tmpPath)
				}
				err = os.Remove(tmpPath)
				if err != nil {
					log.Error().Err(err).Msg("Failed to delete temporary file")
				}
			}

			document := evt.Message.GetDocumentMessage()
			if document != nil {
				tmpDirectory := filepath.Join("/tmp", "user_"+txtid)
				errDir := os.MkdirAll(tmpDirectory, 0751)
				if errDir != nil {
					log.Error().Err(errDir).Msg("Could not create temporary directory")
					return
				}
				data, err := mycli.WAClient.Download(context.Background(), document)
				if err != nil {
					log.Error().Err(err).Msg("Failed to download document")
					return
				}
				extension := ""
				exts, err := mime.ExtensionsByType(document.GetMimetype())
				if err == nil && len(exts) > 0 {
					extension = exts[0]
				} else {
					filename := document.FileName
					if filename != nil {
						extension = filepath.Ext(*filename)
					} else {
						extension = ".bin"
					}
				}
				tmpPath := filepath.Join(tmpDirectory, evt.Info.ID+extension)
				err = os.WriteFile(tmpPath, data, 0600)
				if err != nil {
					log.Error().Err(err).Msg("Failed to save document to temporary file")
					return
				}
				if s3Config.Enabled == "true" && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
					isIncoming := evt.Info.IsFromMe == false
					contactJID := evt.Info.Sender.String()
					if evt.Info.IsGroup {
						contactJID = evt.Info.Chat.String()
					}
					s3Data, err := GetS3Manager().ProcessMediaForS3(
						context.Background(),
						txtid,
						contactJID,
						evt.Info.ID,
						data,
						document.GetMimetype(),
						filepath.Base(tmpPath),
						isIncoming,
					)
					if err != nil {
						log.Error().Err(err).Msg("Failed to upload document to S3")
					} else {
						postmap["s3"] = s3Data
					}
				}
				if s3Config.MediaDelivery == "base64" || s3Config.MediaDelivery == "both" {
					base64String, mimeType, err := fileToBase64(tmpPath)
					if err != nil {
						log.Error().Err(err).Msg("Failed to convert document to base64")
						return
					}
					postmap["base64"] = base64String
					postmap["mimeType"] = mimeType
					postmap["fileName"] = filepath.Base(tmpPath)
				}
				err = os.Remove(tmpPath)
				if err != nil {
					log.Error().Err(err).Msg("Failed to delete temporary file")
				}
			}

			video := evt.Message.GetVideoMessage()
			if video != nil {
				tmpDirectory := filepath.Join("/tmp", "user_"+txtid)
				errDir := os.MkdirAll(tmpDirectory, 0751)
				if errDir != nil {
					log.Error().Err(errDir).Msg("Could not create temporary directory")
					return
				}
				data, err := mycli.WAClient.Download(context.Background(), video)
				if err != nil {
					log.Error().Err(err).Msg("Failed to download video")
					return
				}
				exts, _ := mime.ExtensionsByType(video.GetMimetype())
				tmpPath := filepath.Join(tmpDirectory, evt.Info.ID+exts[0])
				err = os.WriteFile(tmpPath, data, 0600)
				if err != nil {
					log.Error().Err(err).Msg("Failed to save video to temporary file")
					return
				}
				if s3Config.Enabled == "true" && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
					isIncoming := evt.Info.IsFromMe == false
					contactJID := evt.Info.Sender.String()
					if evt.Info.IsGroup {
						contactJID = evt.Info.Chat.String()
					}
					s3Data, err := GetS3Manager().ProcessMediaForS3(
						context.Background(),
						txtid,
						contactJID,
						evt.Info.ID,
						data,
						video.GetMimetype(),
						filepath.Base(tmpPath),
						isIncoming,
					)
					if err != nil {
						log.Error().Err(err).Msg("Failed to upload video to S3")
					} else {
						postmap["s3"] = s3Data
					}
				}
				if s3Config.MediaDelivery == "base64" || s3Config.MediaDelivery == "both" {
					base64String, mimeType, err := fileToBase64(tmpPath)
					if err != nil {
						log.Error().Err(err).Msg("Failed to convert video to base64")
						return
					}
					postmap["base64"] = base64String
					postmap["mimeType"] = mimeType
					postmap["fileName"] = filepath.Base(tmpPath)
				}
				err = os.Remove(tmpPath)
				if err != nil {
					log.Error().Err(err).Msg("Failed to delete temporary file")
				}
			}

			sticker := evt.Message.GetStickerMessage()
			if sticker != nil {
				tmpDirectory := filepath.Join("/tmp", "user_"+txtid)
				errDir := os.MkdirAll(tmpDirectory, 0751)
				if errDir != nil {
					log.Error().Err(errDir).Msg("Could not create temporary directory")
					return
				}

				// baixa o sticker usando a interface DownloadableMessage
				data, err := mycli.WAClient.Download(context.Background(), sticker)
				if err != nil {
					log.Error().Err(err).Msg("Failed to download sticker")
					return
				}

				// tenta inferir extensão pelo mimetype; fallback para .webp
				exts, _ := mime.ExtensionsByType(sticker.GetMimetype())
				ext := ".webp"
				if len(exts) > 0 && exts[0] != "" {
					ext = exts[0]
				}

				tmpPath := filepath.Join(tmpDirectory, evt.Info.ID+ext)
				if err := os.WriteFile(tmpPath, data, 0600); err != nil {
					log.Error().Err(err).Msg("Failed to save sticker to temporary file")
					return
				}

				// se usar S3 (mesmo fluxo das outras mídias)
				if s3Config.Enabled == "true" && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
					isIncoming := evt.Info.IsFromMe == false
					contactJID := evt.Info.Sender.String()
					if evt.Info.IsGroup {
						contactJID = evt.Info.Chat.String()
					}
					s3Data, err := GetS3Manager().ProcessMediaForS3(
						context.Background(),
						txtid,
						contactJID,
						evt.Info.ID,
						data,
						sticker.GetMimetype(),
						filepath.Base(tmpPath),
						isIncoming,
					)
					if err != nil {
						log.Error().Err(err).Msg("Failed to upload sticker to S3")
					} else {
						postmap["s3"] = s3Data
					}
				}

				// base64 (mesmo contrato de saída das outras mídias)
				if s3Config.MediaDelivery == "base64" || s3Config.MediaDelivery == "both" {
					base64String, mimeType, err := fileToBase64(tmpPath)
					if err != nil {
						log.Error().Err(err).Msg("Failed to convert sticker to base64")
						return
					}
					postmap["base64"] = base64String
					postmap["mimeType"] = mimeType
					postmap["fileName"] = filepath.Base(tmpPath)
				}

				// metadados úteis (opcional, mas práticos)
				postmap["isSticker"] = true
				postmap["stickerAnimated"] = sticker.GetIsAnimated()

				if err := os.Remove(tmpPath); err != nil {
					log.Error().Err(err).Msg("Failed to delete temporary file")
				}
			}
		}

	case *events.Receipt:
		postmap["type"] = "ReadReceipt"
		dowebhook = 1
		if evt.Type == types.ReceiptTypeRead || evt.Type == types.ReceiptTypeReadSelf {
			log.Info().Strs("id", evt.MessageIDs).Str("source", evt.SourceString()).Str("timestamp", fmt.Sprintf("%v", evt.Timestamp)).Msg("Message was read")
			if evt.Type == types.ReceiptTypeRead {
				postmap["state"] = "Read"
			} else {
				postmap["state"] = "ReadSelf"
			}
		} else if evt.Type == types.ReceiptTypeDelivered {
			postmap["state"] = "Delivered"
			log.Info().Str("id", evt.MessageIDs[0]).Str("source", evt.SourceString()).Str("timestamp", fmt.Sprintf("%v", evt.Timestamp)).Msg("Message delivered")
		} else {
			return
		}
	case *events.Presence:
		postmap["type"] = "Presence"
		dowebhook = 1
		if evt.Unavailable {
			postmap["state"] = "offline"
			if evt.LastSeen.IsZero() {
				log.Info().Str("from", evt.From.String()).Msg("User is now offline")
			} else {
				log.Info().Str("from", evt.From.String()).Str("lastSeen", fmt.Sprintf("%v", evt.LastSeen)).Msg("User is now offline")
			}
		} else {
			postmap["state"] = "online"
			log.Info().Str("from", evt.From.String()).Msg("User is now online")
		}
	case *events.HistorySync:
		postmap["type"] = "HistorySync"
		dowebhook = 1
	case *events.AppState:
		log.Info().Str("index", fmt.Sprintf("%+v", evt.Index)).Str("actionValue", fmt.Sprintf("%+v", evt.SyncActionValue)).Msg("App state event received")
	case *events.LoggedOut:
		postmap["type"] = "Logged Out"
		dowebhook = 1
		log.Info().Str("reason", evt.Reason.String()).Msg("Logged out")
		killchannel[mycli.userID] <- true
		sqlStmt := `UPDATE users SET connected=0 WHERE id=$1`
		_, err := mycli.db.Exec(sqlStmt, mycli.userID)
		if err != nil {
			log.Error().Err(err).Msg(sqlStmt)
			return
		}
	case *events.ChatPresence:
		postmap["type"] = "ChatPresence"
		dowebhook = 1
		log.Info().Str("state", fmt.Sprintf("%s", evt.State)).Str("media", fmt.Sprintf("%s", evt.Media)).Str("chat", evt.MessageSource.Chat.String()).Str("sender", evt.MessageSource.Sender.String()).Msg("Chat Presence received")
	case *events.CallOffer:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call offer")
	case *events.CallAccept:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call accept")
	case *events.CallTerminate:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call terminate")
	case *events.CallOfferNotice:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call offer notice")
	case *events.CallRelayLatency:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call relay latency")
	case *events.Disconnected:
		postmap["type"] = "Disconnected"
		dowebhook = 1
		log.Info().Str("reason", fmt.Sprintf("%+v", evt)).Msg("Disconnected from Whatsapp")
	case *events.ConnectFailure:
		postmap["type"] = "ConnectFailure"
		dowebhook = 1
		log.Error().Str("reason", fmt.Sprintf("%+v", evt)).Msg("Failed to connect to Whatsapp")
	default:
		log.Warn().Str("event", fmt.Sprintf("%+v", evt)).Msg("Unhandled event")
	}

	if dowebhook == 1 {
		sendEventWithWebHook(mycli, postmap, path)
	}
}
