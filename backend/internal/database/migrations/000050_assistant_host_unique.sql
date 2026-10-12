-- Two shops must never share an assistant: the LINE front gate would pass one shop's messages to the other shop's assistant.
create unique index tenant_assistant_settings_assistant_host_uidx on tenant_assistant_settings (assistant_host) where assistant_host <> '';
