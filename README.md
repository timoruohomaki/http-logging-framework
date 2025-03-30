# HTTP logging framework
Logging functionality for HTTP listeners using Apache Common Log Format using Zap

Key features:
- Two log format options (Common and Combined)
- A configuration structure for Apache-style logging
- A secure file creation and permission setting function
- A function to secure rotated log files
- The response wrapper to capture status and size
- Log formatting for both Common and Combined formats
- The HTTP middleware to handle the actual logging

> [!NOTE]
> [Apache logging format documentation](https://httpd.apache.org/docs/2.4/logs.html)
