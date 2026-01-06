# How to use webhooks?
[link](https://www.chatwoot.com/docs/product/features/webhooks)

Last updated on May 09, 2025


Webhooks are HTTP callbacks that are set up for each account. These are triggered when actions such as creating a message occur in Chatwoot. Multiple webhooks can be created for a single account.

## How to add a webhook? 

**Step 1.** Go to **Settings → Integrations → Webhooks**. Click on the "Configure" button.

**Step 2.** Click on the "Add new webhook" button. A modal will open up. Here, input the URL to which the POST request should be sent. Next, you need to select the events you want to subscribe. This option would allow you to only listen to the relevant events in Chatwoot.

Chatwoot will send a POST request with the following payload to the configured URLs for various updates in your account.

### A sample webhook payload 

```
{

  "event": "message_created", // The name of the event
  "id": "1", // Message ID
  "content": "Hi", // Content of the message
  "created_at": "2020-03-03 13:05:57 UTC", // Time at which the message was sent
  "message_type": "incoming", // This will have a type incoming, outgoing or template. The user from the widget sends incoming messages, and the agent sends outgoing messages to the user.
  "content_type": "enum", // This is an enum, it can be input_select, cards, form or text. The message_type will be template if content_type is one og these. Default value is text
  "content_attributes": {} // This will an object, different values are defined below
  "source_id": "", // This would the external id if the inbox is a Twitter or Facebook integration.
  "sender": { // This would provide the details of the agent who sent this message
    "id": "1",
    "name": "Agent",
    "email": "[email protected]"
  },
  "contact": { // This would provide the details of the user who sent this message
    "id": "1",
    "name": "contact-name"
  },
  "conversation": { // This would provide the details of the conversation
    "display_id": "1", // This is the ID of the conversation which you can see in the dashboard.
    "additional_attributes": {
      "browser": {
        "device_name": "Macbook",
        "browser_name": "Chrome",
        "platform_name": "Macintosh",
        "browser_version": "80.0.3987.122",
        "platform_version": "10.15.2"
      },
      "referer": "<http://www.chatwoot.com>",
      "initiated_at": "Tue Mar 03 2020 18:37:38 GMT-0700 (Mountain Standard Time)"
    }
  },
  "account": { // This would provide the details of the account
    "id": "1",
    "name": "Chatwoot",
  }
}
```

## Supported webhook events in Chatwoot 

Each event has its payload structure based on the type of model they are acting on. The following section describes the main objects we use in Chatwoot and their attributes.

## Objects 

An event payload may include any of the following objects. The various types of objects supported by Chatwoot are listed below.

**Account**

```
{
  "id": "integer",
  "name": "string"
}
```

**Inbox**

```
{
"id": "integer",
"name": "string"
}
```

**Contact**

```
{
  "id": "integer",
  "name": "string",
  "avatar": "string",
  "type": "contact",
  "account": {
    // <...Account Object>
  }
}
```

**User**

```
{
  "id": "integer",
  "name": "string",
  "email": "string",
  "type": "user"
}
```

**Conversation**

```
{
  "additional_attributes": {
    "browser": {
      "device_name": "string",
      "browser_name": "string",
      "platform_name": "string",
      "browser_version": "string",
      "platform_version": "string"
    },
    "referer": "string",
    "initiated_at": {
      "timestamp": "iso-datetime"
    }
  },
  "can_reply": "boolean",
  "channel": "string",
  "id": "integer",
  "inbox_id": "integer",
  "contact_inbox": {
    "id": "integer",
    "contact_id": "integer",
    "inbox_id": "integer",
    "source_id": "string",
    "created_at": "datetime",
    "updated_at": "datetime",
    "hmac_verified": "boolean"
  },
  "messages": ["Array of message objects"],
  "meta": {
    "sender": {
      // Contact Object
    },
    "assignee": {
      // User Object
    }
  },
  "status": "string",
  "unread_count": "integer",
  "agent_last_seen_at": "unix-timestamp",
  "contact_last_seen_at": "unix-timestamp",
  "timestamp": "unix-timestamp",
  "account_id": "integer"
}
```

**Message**

```
{
  "id": "integer",
  "content": "string",
  "message_type": "integer",
  "created_at": "unix-timestamp",
  "private": "boolean",
  "source_id": "string / null",
  "content_type": "string",
  "content_attributes": "object",
  "sender": {
    "type": "string - contact/user"
    // User or Contact Object
  },
  "account": {
    // Account Object
  },
  "conversation": {
    // Conversation Object
  },
  "inbox": {
    // Inbox Object
  }
}
```

**A sample webhook payload**

```
{
  "event": "event_name"
  // Attributes related to the event
}
```

## Webhook Events

Chatwoot supports the following webhook events. You can subscribe to them while configuring a webhook in the dashboard or using the API.

### conversation\_created

This event will be triggered when a new conversation is created in the account. The payload for the event is as follows.

```
{
  "event": "conversation_created"
  // <...Conversation Attributes>
}
```

### conversation\_updated

This event will be triggered when there is a change in any of the attributes in the conversation.

```
{
  "event": "conversation_updated",
  "changed_attributes": [\
    {\
      "<attribute_name>": {\
        "current_value": "",\
        "previous_value": ""\
      }\
    }\
  ]
  // <...Conversation Attributes>
}
```

### conversation\_status\_changed 

This event will be triggered when the status of the conversation is changed.

Note: If you are using agent bot APIs instead of webhooks, this event is not supported yet.

```
{
  "event": "conversation_status_changed"
  // <...Conversation Attributes>
}
```

### message\_created 

This event will be triggered when a message is created in a conversation. The payload for the event is as follows.

```
{
  "event": "message_created"
  // <...Message Attributes>
}
```

### message\_updated 

This event will be triggered when a message is updated in a conversation. The payload for the event is as follows.

```
{
  "event": "message_updated"
  // <...Message Attributes>
}
```

### webwidget\_triggered 

This event will be triggered when the end user opens the live-chat widget.

```
{
  "event": "webwidget_triggered",
  "id": "",
  "contact": {
    // <...Contact Object>
  },
  "inbox": {
    // <...Inbox Object>
  },
  "account": {
    // <...Account Object>
  },
  "current_conversation": {
    // <...Conversation Object>
  },
  "source_id": "string",
  "event_info": {
    "initiated_at": {
      "timestamp": "date-string"
    },
    "referer": "string",
    "widget_language": "string",
    "browser_language": "string",
    "browser": {
      "browser_name": "string",
      "browser_version": "string",
      "device_name": "string",
      "platform_name": "string",
      "platform_version": "string"
    }
  }
}
```

### conversation\_typing\_on 

This event is triggered when an agent starts typing in a conversation. It could be either a private note or a message to the customer. You can use the `is_private` flag to distinguish between the two.

```
{
  "event": "conversation_typing_on",
  "conversation": { ...<Conversation Object> },
  "user": { ... <User / AgentBot / Captain Object> },
  "is_private": true
}
```

### conversation\_typing\_off 

This event is triggered when an agent stops typing or when they leave the conversation window.

```
{
  "event": "conversation_typing_off",
  "conversation": { ...<Conversation Object> },
  "user": { ... <User / AgentBot / Captain Object> },
  "is_private": true
}
```
