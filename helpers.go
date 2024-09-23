package main

import (
	"fmt"
	"strconv"
)

func Find(slice []string, val string) bool {
    for _, item := range slice {
        if item == val {
            return true
        }
    }
    return false
}

// Update entry in User map
func updateUserInfo(values interface{}, field string, value string) interface{} {
    log.Debug().Str("field",field).Str("value",value).Msg("User info updated")
    values.(Values).m[field] = value
    return values
}

// webhook for regular messages
func callHook(myurl string, payload map[string]string, id int) {
    log.Info().Str("url",myurl).Msg("Sending POST to client "+strconv.Itoa(id))

    // Log the payload map
    log.Debug().Msg("Payload:")
    for key, value := range payload {
        log.Debug().Str(key, value).Msg("")
    }

    _, err := clientHttp[id].R().SetFormData(payload).Post(myurl)
    if err != nil {
        log.Debug().Str("error",err.Error())
    }
}

// webhook for messages with file attachments
func callHookFile(myurl string, payload map[string]string, id int, file string) error {
    log.Info().Str("file", file).Str("url", myurl).Msg("Sending POST")

    // Criar um novo mapa para o payload final
    finalPayload := make(map[string]string)
    for k, v := range payload {
        finalPayload[k] = v
    }

    // Adicionar o arquivo ao payload
    finalPayload["file"] = file

    log.Debug().Interface("finalPayload", finalPayload).Msg("Final payload to be sent")

    resp, err := clientHttp[id].R().
        SetFiles(map[string]string{
            "file": file,
        }).
        SetFormData(finalPayload).
        Post(myurl)

    if err != nil {
        log.Error().Err(err).Str("url", myurl).Msg("Failed to send POST request")
        return fmt.Errorf("failed to send POST request: %w", err)
    }

    // Log do payload enviado
    log.Debug().Interface("payload", finalPayload).Msg("Payload sent to webhook")

    // Optionally, you can log the response status and body
    log.Info().Int("status", resp.StatusCode()).Str("body", string(resp.Body())).Msg("POST request completed")

    return nil
}