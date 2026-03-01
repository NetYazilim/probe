# Probe - Çok Protokollü Health Check

[English README](README.md)

Go ile yazılmış çok protokollü sağlık kontrolü kütüphanesi. Öncelikle projelerinize paket olarak import edin; basit kontroller için beraberindeki CLI aracını kullanın.

> **Not:** Bu depodaki CLI aracı, temel olarak kütüphanenin yeteneklerini göstermek ve basit kontroller yapmak için tasarlanmıştır. Bu kütüphaneyi temel alan tam kapsamlı ayrıntılı log aracı için lütfen **[NetYazilim/izle](https://github.com/NetYazilim/izle)** projesini inceleyin.

## Özellikler

- **ICMP Ping Probe** - ICMP Echo kullanarak ana bilgisayar erişilebilirliğini kontrol edin (IP adresleri ve hostname'leri destekler; Linux'ta IPv6 desteği vardır).
- **TCP Probe** - Port bağlantısı ve bağlantı bilgilerini doğrulayın (IPv4 ve IPv6 hedeflerde çalışır).
- **HTTP/HTTPS Probe** - Web servisi sağlığını durum kodu kontrolleriyle doğrulayın (IPv4/IPv6 uç noktaları otomatik desteklenir).
- **TLS/SSL Probe** - SSL/TLS sertifika bilgilerini (Bitiş Tarihi, Kalan Gün Sayısı, Yayınlayan vb.) doğrudan sunucudan alın ve doğrulayın (IPv4 veya IPv6 üzerinden erişilebilir).
- **Harici Go kütüphane bağımlılığı yoktur - yalnızca Go standart kütüphanesi kullanılır.**

## Platform Desteği

- **Linux** - UDP tabanlı ICMP kullanır (unprivileged ping için kernel desteği gerekir) ve IPv4 ile IPv6 hedeflerini destekler.
- **Windows** - Win32 ICMP API kullanır (özel yetki gerekmez) ve şu anda ICMP ping için yalnızca IPv4 desteklenir.

### Linux ICMP Ping Konfigürasyonu

Linux'ta, çekirdek unprivileged ICMP ping'e izin vermelidir. Kontrol etmek için:

```bash
cat /proc/sys/net/ipv4/ping_group_range
```

**Çıktı anlamları:**
- `1 0` - ICMP ping unprivileged kullanıcılar için kapalı
- `0 2147483647` - Tüm kullanıcılar root olmadan ICMP ping yapabilir

**Unprivileged ICMP ping'i etkinleştirmek için:**

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
```

**Değişikliği kalıcı hale getirmek için:**

```bash
echo "net.ipv4.ping_group_range = 0 2147483647" | sudo tee /etc/sysctl.d/99-ping.conf
sudo sysctl -p /etc/sysctl.d/99-ping.conf
```

## Kurulum

```bash
go get "github.com/netyazilim/probe"
```

## Kullanım

```bash
# Yardım görüntüle
./probe

# ICMP Ping (IP Adresleri veya Hostname'ler)
./probe ping 8.8.8.8
./probe ping google.com
./probe -attempts 5 ping 8.8.8.8
./probe -timeout 2s ping google.com
./probe -loop 5s ping 8.8.8.8                    # Her 5 saniyede çalıştır

# TCP Probe (Port Bağlantısı)
./probe tcp example.com:22
./probe tcp google.com:443
./probe -attempts 5 tcp google.com:443
./probe -loop 10s tcp google.com:443             # Her 10 saniyede çalıştır

# HTTP/HTTPS (URL'ler)
./probe http https://google.com
./probe -timeout 3s http https://example.com
./probe -loop 1m http https://google.com         # Her 1 dakikada çalıştır

# TLS/SSL Sertifika Bilgisi
./probe tls google.com:443
./probe -timeout 10s tls example.com:443
./probe -loop 30m tls google.com:443             # Her 30 dakikada çalıştır
```

## Parametreler

```
-attempts int     Maksimum deneme sayısı (varsayılan: 3)
-timeout duration Deneme başına zaman aşımı (varsayılan: ping/tcp/http için 1s, tls için 5s)
-loop duration    Loop aralığı (0 = bir kez çalıştır, örn: 5s, 1m, 10s)
```

## Komutlar

```
ping               ICMP ping probe'u (IP adresleri ve hostname'leri destekler)
tcp                TCP port bağlantısı kontrolü (host:port formatı)
http               HTTP/HTTPS durum kontrolü (URL formatı)
tls                TLS/SSL sertifika bilgisi (host:port formatı)
```

## Zaman Aşımı Konfigürasyonu

- **ICMP Ping**: Deneme başına 1 saniye
- **TCP**: Deneme başına 1 saniye
- **HTTP/HTTPS**: Deneme başına 1 saniye
- **TLS/SSL**: Deneme başına 5 saniye

Her probe varsayılan olarak 3 yeniden deneme girişimini destekler.

## Docker Kullanımı

```bash
# Docker image'ını oluştur
docker build -t probe .

# Container'ı çalıştır
docker run --rm probe ping 8.8.8.8
docker run --rm probe tcp google.com:443
docker run --rm probe http https://google.com
docker run --rm probe tls google.com:443

# Özel deneme sayısı ve timeout ile çalıştır
docker run --rm probe ping -attempts 5 8.8.8.8
docker run --rm probe tcp -timeout 2s google.com:443

# Loop modunda çalıştır
docker run --rm probe ping -loop 10s 8.8.8.8
```

### Docker Notu
Container, yetkisiz bir kullanıcı (`probeuser`) olarak çalışır.

## Çıktı Örnekleri

### ICMP Ping
```
Target: 8.8.8.8
Attempt: 3
Success: true
Duration: 45.32 ms
```

### TCP Probe
```
Host: google.com
Port: 443
Attempt: 3
Success: true
Duration: 125.45 ms
Local Address: 192.168.1.100:52341
Remote Address: 142.251.32.14:443
```

### HTTP/HTTPS
```
URL: https://google.com
Attempt: 3
Success: true
Status Code: 200
Duration: 125.45 ms
```

### TLS/SSL
```
Host: google.com
Attempt: 3
Success: true
Duration: 235.67 ms
Subject: CN=www.google.com
Issuer: CN=Google Internet Authority G3
Expires At: 2025-12-15 23:59:59
Days Until Expiry: 310
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
```

## Bağımlılıklar

Bu paket yalnızca Go standart kütüphanesi paketlerini kullanır:
Harici bağımlılık gerekmez.

## Gereksinimler

- Go 1.20 veya üstü
- Linux'ta ICMP Ping için:
  - Kernel desteğini kontrol edin: `cat /proc/sys/net/ipv4/ping_group_range`
  - Çekirdekte unprivileged ICMP etkinleştirilmiş olmalı
  - Etkinleştirmek için: `sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"`
- TCP ve TLS probe'ları için internet bağlantısı

## Hata İşleme

Tüm probe'lar retry mantığı içerir (varsayılan olarak 3 deneme) ve denemeler arasında 500ms gecikmeler vardır. Sorun giderme için detaylı hata mesajları sağlanır.
