# Bulk SMS Sending

The purpose of this method is to send the same message to a group of recipients. For single-message sending, use this method as well. In this method, you can send an SMS request to a maximum of **10,000** people.

## URL Structure of This Method

You must use the **POST** method to send to this endpoint:

```text
https://sms.najva.com/v2/sms/send
```

## Input Parameters

| Parameter  |   Type | Required | Description                                                                                      |
| ---------- | -----: | -------: | ------------------------------------------------------------------------------------------------ |
| `receptor` | String | Required | Specifies the recipients’ phone numbers as a list of numbers.                                    |
| `message`  | String | Required | The text of the message to be sent.                                                              |
| `sender`   | String | Required | The sender line number. For Bale, this is the same as the Bale ID.                               |
| `file_id`  | String | Optional | The ID of the file you have previously uploaded for Bale. Refer to the Bale file upload section. |

## Output Parameters

| Parameter    |     Type | Description                                     |
| ------------ | -------: | ----------------------------------------------- |
| `messageid`  |     Long | Unique message identifier.                      |
| `message`    |   String | Text of the sent message.                       |
| `status`     |  Integer | Message sending status.                         |
| `statustext` |   String | Text description of the message sending status. |
| `sender`     |   String | Sender number.                                  |
| `receptor`   |   String | Recipient number.                               |
| `date`       | UnixTime | Message sending time.                           |
| `cost`       |  Integer | SMS cost.                                       |

## Notes

* To check the message status, refer to the **message status table** at the end of the document.
* The recipient limit is **10,000** people.

## Error Table

| Error Code | Description                                   |
| ---------: | --------------------------------------------- |
|      `400` | The input parameters are invalid.             |
|      `414` | The number of recipients is more than 10,000. |
|      `418` | Your account balance is insufficient.         |


# Peer-to-Peer SMS Sending

The purpose of this method is to send bulk SMS messages in a peer-to-peer format, meaning the sent message text may differ for each recipient. In this method, you can send an SMS request to a maximum of **10,000** people.

## URL Structure of This Method

You must use the **POST** method to send to this endpoint:

```text
https://sms.najva.com/v2/sms/send-p2p
```

## Input Parameters

| Parameter  |   Type | Required | Description                                                                                      |
| ---------- | -----: | -------: | ------------------------------------------------------------------------------------------------ |
| `receptor` | String | Required | Specifies the recipients’ phone numbers as a list of numbers.                                    |
| `message`  | String | Required | The text of the message to be sent.                                                              |
| `sender`   | String | Optional | The sender line number. For Bale, this is the same as the Bale ID.                               |
| `file_id`  | String | Optional | The ID of the file you have previously uploaded for Bale. Refer to the Bale file upload section. |

## Output Parameters

| Parameter    |     Type | Description                                     |
| ------------ | -------: | ----------------------------------------------- |
| `messageid`  |     Long | Unique message identifier.                      |
| `message`    |   String | Text of the sent message.                       |
| `status`     |  Integer | Message sending status.                         |
| `statustext` |   String | Text description of the message sending status. |
| `sender`     |   String | Sender number.                                  |
| `receptor`   |   String | Recipient number.                               |
| `date`       | UnixTime | Message sending time.                           |
| `cost`       |  Integer | SMS cost.                                       |

## Notes

* To check the message status, refer to the **message status table** at the end of the document.
* The recipient limit is **10,000** people.

## Error Table

| Error Code | Description                                   |
| ---------: | --------------------------------------------- |
|      `400` | The input parameters are invalid.             |
|      `414` | The number of recipients is more than 10,000. |
|      `418` | Your account balance is insufficient.         |


# File Upload for Bale

To send a file in Bale, you first need to upload the desired file and then send its ID in your requests.

## URL Structure of This Method

You must use the **POST** method to send to this endpoint:

```text
https://sms.najva.com/upload-file/bale
```

## Input Parameters

| Parameter |                       Type | Required | Description                          |
| --------- | -------------------------: | -------: | ------------------------------------ |
| `file`    | File `multipart/form-data` | Required | The file you want to upload in Bale. |

## Output Parameters

| Parameter |          Type | Description                                                   |
| --------- | ------------: | ------------------------------------------------------------- |
| `file_id` | String `UUID` | Unique ID of the uploaded file, used for sending the message. |

## Error Table

| Error Code | Description                                                                                               |
| ---------: | --------------------------------------------------------------------------------------------------------- |
|      `400` | The file extension is not allowed. Allowed extensions: `jpeg`, `jpg`, `png`, `gif`, `opus`, `ogg`, `mp4`. |
|      `413` | The file size exceeds the allowed limit. Maximum allowed size: **15 MB**.                                 |


### Message Status Table

| Status Code | Description                                                                                                                                                                           |
| ----------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
|         `1` | In the sending queue.                                                                                                                                                                 |
|         `2` | Scheduled.                                                                                                                                                                            |
|         `4` | Sent to the telecom operator.                                                                                                                                                         |
|         `6` | Error sending the message to the specified phone number.                                                                                                                              |
|        `10` | The SMS has been delivered to the recipient.                                                                                                                                          |
|        `11` | A problem occurred in SMS delivery. The recipient being unavailable or their phone being turned off are among the possible reasons for this status.                                   |
|        `13` | SMS sending has been canceled.                                                                                                                                                        |
|        `14` | The recipient has blocked receiving this SMS. The phone number or promotional messages may have been blocked by the recipient. The cost of this SMS will be returned to your account. |
|       `100` | The message ID is invalid.                                                                                                                                                            |

# Receiving Sending Status

After messages are sent through the web service, their status will be **in queue**. Then, they will be sent to the telecom operator, and their sending status to the operator will be received. After that, their status is continuously checked until they reach the final status in the system.

Use this method to check the **Delivery** status of a message. To use this method, you must send the unique `messageid` of each message, which you received in the output after sending the SMS, as the input parameter.

On each execution of this method, you can receive the status of a maximum of **1,000** messages.

## URL Structure of This Method

You must use the **POST** method to send to this endpoint:

```text id="ohzfb0"
https://sms.najva.com/v2/sms/status
```

## Input Parameters

| Parameter    |       Type | Required | Description                                                     |
| ------------ | ---------: | -------: | --------------------------------------------------------------- |
| `messageids` | List[Long] | Required | List of `messageid` values whose statuses you want to retrieve. |

## Output Parameters

| Parameter    |    Type | Description                                     |
| ------------ | ------: | ----------------------------------------------- |
| `messageid`  |    Long | Unique message identifier.                      |
| `status`     | Integer | Message sending status.                         |
| `statustext` |  String | Text description of the message sending status. |

## Notes

* To view the complete list of statuses, refer to the **message status table** at the end of the document.
* If the message `messageid` is invalid, the `status` field value will be `100`.

## Error Table

| Error Code | Description                                          |
| ---------: | ---------------------------------------------------- |
|      `400` | The input parameters are invalid.                    |
|      `414` | The number of `messageId` values is more than 1,000. |
