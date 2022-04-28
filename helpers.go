package main

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
	log.Info().Str("url",myurl).Msg("Sending POST")
	clientHttp[id].R().SetFormData(payload).Post(myurl)
}

// webhook for messages with file attachments
func callHookFile(myurl string, payload map[string]string, id int, file string) {
	log.Info().Str("file",file).Str("url",myurl).Msg("Sending POST")
	clientHttp[id].R().SetFiles(map[string]string{
		"file": file,
	}).SetFormData(payload).Post(myurl)
}


