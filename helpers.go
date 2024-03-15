package main

import ( "strconv" )

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
    /*
    ti := resp.Request.TraceInfo()
    log.Debug().Msg("  DNSLookup     :"+ ti.DNSLookup.String())
    log.Debug().Msg("  ConnTime      :"+ ti.ConnTime.String())
    log.Debug().Msg("  TCPConnTime   :"+ ti.TCPConnTime.String())
    log.Debug().Msg("  TLSHandshake  :"+ ti.TLSHandshake.String())
    log.Debug().Msg("  ServerTime    :"+ ti.ServerTime.String())
    log.Debug().Msg("  ResponseTime  :"+ ti.ResponseTime.String())
    log.Debug().Msg("  TotalTime     :"+ ti.TotalTime.String())
    log.Debug().Msg("  IsConnReused  :"+ strconv.FormatBool(ti.IsConnReused))
    log.Debug().Msg("  IsConnWasIdle :"+ strconv.FormatBool(ti.IsConnWasIdle))
    log.Debug().Msg("  ConnIdleTime  :"+ ti.ConnIdleTime.String())
    log.Debug().Msg("  RequestAttempt:"+ strconv.Itoa(ti.RequestAttempt))
    */
}

// webhook for messages with file attachments
func callHookFile(myurl string, payload map[string]string, id int, file string) {
    log.Info().Str("file",file).Str("url",myurl).Msg("Sending POST")
    clientHttp[id].R().SetFiles(map[string]string{
        "file": file,
    }).SetFormData(payload).Post(myurl)
}

