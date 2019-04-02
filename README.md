# Docs2Email

_Command line utility for sending Google Docs by Email_

**Configuring Google APIs:**

- Visit the [Google Developer Console](https://console.developers.google.com/start/api?id=drive) and create a new project
- Create credentials -> OAuth client ID
- Select "Other"
- Download the credentials and save as `client_id.json` in the root of this repo
- Then, under "Library" search for "Drive" and enable the Drive API for this project

On first run of docs2email you'll be prompted throug hthe google oauth flow.

**Usage:**

```bash
go get -u github.com/dpup/docs2email
cd github.com/dpup/docs2email
go run *.go \
  --sendgrid-api-key=$SEND_GRID_API_KEY \
  --from="Bill Lumbergh <bill@initech.com>" \
  --test="Bill Lumbergh <billyboy1999@gmail.com>" \
  --to="Peter Gibbons <peter@initech.com>, Milton Waddams <temp43@initech.com>, Michael Bolton <bolton@initech.com>" \
  --cc="Tom Smykowski <tom@initech.com>" \
  --bcc="Bob Slydell <bob@downsize.r.us>" \
  --subject="Quarterly TPS Report" \
  --file-id="1inglnJi363gY9-1lgLYBCc1gi-iEbwpNfXndxOQNrOY"
```

**What happens:**

- You will be prompted to login via Google and authorize drive.
- Copy/paste the access token when prompted, this will be cached locally.
- Your doc is downloaded as a zip, parsed and cleaned up.
- A test email is sent to the address specified in the `--test` flag.
- Check the email looks good, then type "yes".
- The email will be resent with the full TO, CC, and BCC lists specified in the flags.

[Get a Sendgrid API Key](https://app.sendgrid.com/settings/api_keys).

## Contributing

Questions, comments, bug reports, and pull requests are all welcome. Submit them
[on the project issue tracker](https://github.com/dpup/docs2email/new).

## License

Copyright 2018 [Daniel Pupius](http://pupius.co.uk). Licensed under the
[Apache License, Version 2.0](http://www.apache.org/licenses/LICENSE-2.0).
