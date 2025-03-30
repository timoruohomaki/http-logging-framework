# HTTP logging framework
Logging functionality for HTTP listeners using Apache Common Log Format using Zap
and Lumberjack (for log rotation)

[Apache logging format documentation](https://httpd.apache.org/docs/2.4/logs.html)

> [!NOTE]
> - Log file permissions need to be set manually, e.g.:
> touch /var/log/apache2/access.log
> sudo chmod 644 /var/log/apache2/access.log
> **TO DO**:
> - Add Combined Log Format as an alternative schema, adds e.g. Referrer element
