package notification

import (
	"fmt"
	"net/smtp"

	log "github.com/sirupsen/logrus"

	"github.com/rodrigo-brito/ninjabot/model"
)

/*
Mail implements email notification service for trading bot alerts and order updates.
It provides SMTP-based email delivery with support for various email providers
including Gmail, Outlook, and custom SMTP servers.

Features:
  • Order status notifications with emoji indicators
  • Error alerts for system issues
  • Customizable SMTP configuration
  • Plain text authentication support
  • Automatic message formatting
*/
type Mail struct {
	auth smtp.Auth  // SMTP authentication credentials

	// SMTP server configuration
	smtpServerPort    int     // SMTP server port (usually 587 for TLS)
	smtpServerAddress string  // SMTP server hostname

	// Email addresses
	to   string  // Recipient email address
	from string  // Sender email address (must match auth credentials)
}

/*
Notify sends a plain text notification email with the specified message content.
The method constructs a properly formatted email message and delivers it via SMTP.

The email format includes:
  • To/From headers with descriptive names
  • Subject line embedded in the message text
  • Plain text body content

Parameters:
  • text: Email content including subject and body
*/
func (t Mail) Notify(text string) {
	// Construct SMTP server address with port
	serverAddress := fmt.Sprintf(
		"%s:%d",
		t.smtpServerAddress,
		t.smtpServerPort)

	// Format email message with headers
	message := fmt.Sprintf(
		`To: "User" <%s>\nFrom: "NinjaBot" <%s>\n%s`,
		t.to,
		t.from,
		text,
	)

	// Send email via SMTP
	err := smtp.SendMail(
		serverAddress,
		t.auth,
		t.from,
		[]string{t.to},
		[]byte(message))
	if err != nil {
		log.
			WithError(err).
			Errorf("notification/mail: couldnt send mail")
	}
}

/*
OnOrder handles order status change notifications by sending formatted email alerts.
Different order statuses trigger distinct email subjects with emoji indicators
for quick visual identification.

Order Status Mapping:
  • FILLED: ✅ (Green checkmark for successful trades)
  • NEW: 🆕 (New emoji for pending orders)
  • CANCELED/REJECTED: ❌ (Red X for failed orders)

The email includes the complete order details for reference.
*/
func (t Mail) OnOrder(order model.Order) {
	title := ""
	switch order.Status {
	case model.OrderStatusTypeFilled:
		title = fmt.Sprintf("✅ ORDER FILLED - %s", order.Pair)
	case model.OrderStatusTypeNew:
		title = fmt.Sprintf("🆕 NEW ORDER - %s", order.Pair)
	case model.OrderStatusTypeCanceled, model.OrderStatusTypeRejected:
		title = fmt.Sprintf("❌ ORDER CANCELED / REJECTED - %s", order.Pair)
	}

	// Format email with subject and order details
	message := fmt.Sprintf("Subject: %s\nOrder %s", title, order)

	t.Notify(message)
}

/*
OnError sends immediate email alerts for system errors and exceptions.
Error notifications use a stop sign emoji (🛑) for high visibility
and include the complete error message for debugging.

This method ensures critical system issues are promptly reported
to administrators for quick resolution.
*/
func (t Mail) OnError(err error) {
	message := fmt.Sprintf("Subject: 🛑 ERROR\nError %s", err)
	t.Notify(message)
}

/*
MailParams contains configuration parameters for email notification setup.
All fields are required for proper SMTP authentication and delivery.
*/
type MailParams struct {
	SMTPServerPort    int    // SMTP server port (typically 587 for TLS)
	SMTPServerAddress string // SMTP hostname (e.g., "smtp.gmail.com")

	To       string // Recipient email address
	From     string // Sender email address (must match password)
	Password string // Email account password or app-specific password
}

/*
NewMail creates a new email notification service with SMTP configuration.
It sets up plain authentication and validates the connection parameters.

For Gmail users:
  • Use "smtp.gmail.com" as SMTPServerAddress
  • Use port 587 for TLS
  • Enable 2FA and use an app-specific password
  • From address must match the authenticated account

Parameters:
  • params: Email configuration including SMTP settings and credentials

Returns configured Mail notifier ready for sending alerts.
*/
func NewMail(params MailParams) Mail {
	return Mail{
		from:              params.From,
		to:                params.To,
		smtpServerPort:    params.SMTPServerPort,
		smtpServerAddress: params.SMTPServerAddress,
		auth: smtp.PlainAuth(
			"",                        // Identity (usually empty)
			params.From,               // Username (email address)
			params.Password,           // Password or app-specific password
			params.SMTPServerAddress,  // SMTP server hostname
		),
	}
}
