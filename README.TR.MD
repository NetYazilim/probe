# Probe - Çok Protokollü Health Check

[English README](README.md)

Go ile yazılmış çok protokollü sağlık kontrolü kütüphanesi. Öncelikle projelerinize paket olarak import edin; basit kontroller için beraberindeki CLI aracını kullanın.

> **Not:** Bu depodaki CLI aracı, temel olarak kütüphanenin yeteneklerini göstermek ve basit kontroller yapmak için tasarlanmıştır. Bu kütüphaneyi temel alan tam kapsamlı ayrıntılı log aracı için lütfen **[NetYazilim/izle](https://github.com/NetYazilim/izle)** projesini inceleyin.

## Özellikler

- **ICMP Ping Probe** - ICMP Echo kullanarak ana bilgisayar erişilebilirliğini kontrol edin (IP adresleri ve hostname'leri destekler; Linux'ta IPv6 desteği vardır).
- **TCP Probe** - Port bağlantısı ve bağlantı bilgilerini doğrulayın (IPv4 ve IPv6 hedeflerde çalışır).
- **HTTP/HTTPS Probe** - Web servisi sağlığını durum kodu kontrolleriyle doğrulayın (IPv4/IPv6 uç noktaları otomatik desteklenir).
- **TLS/SSL Probe** - IPv4 ve IPv6 üzerinde iki modu destekler: strict TLS doğrulaması (`tls`) ve yalnızca sertifika inceleme modu (`tls-cert`). Sertifika-only modunda, sunucu mTLS/istemci sertifikası istese ve tam TLS oturumu kurulmasa bile sunulan sunucu sertifikası okunabilir.
- **Asgari bağımlılık** - `golang.org/x/net` yalnızca Linux'ta ICMP ping için gereklidir; TCP, HTTP ve TLS probe'ları ile Windows ve macOS derlemeleri tek başına Go standart kütüphanesini kullanır.

## Platform Desteği

- **Linux** - UDP tabanlı ICMP kullanır (unprivileged ping için kernel desteği gerekir) ve IPv4 ile IPv6 hedeflerini destekler.
- **Windows** - Win32 ICMP API kullanır (özel yetki gerekmez) ve şu anda ICMP ping için yalnızca IPv4 desteklenir.
- **macOS ve diğer platformlar** - ICMP ping uygulanmamıştır ve açık bir hata döndürür; TCP, HTTP ve TLS probe'ları normal çalışır.

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

> systemd tabanlı bir dağıtımda bu genellikle zaten varsayılandır: systemd,
> `sysctl.d/50-default.conf` dosyasında *"ping(8) without CAP_NET_ADMIN and
> CAP_NET_RAW"* yorumuyla `net.ipv4.ping_group_range = 0 2147483647` satırını
> getirir. Bu komuta asıl ihtiyaç duyacağınız yer, `systemd-sysctl`
> çalıştırmayan container'lar ve minimal imajlardır; oralarda kernel'in
> varsayılanı olan `1 0` (kapalı) geçerli kalır. Varsaymak yerine önce kontrol
> edin.

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
./probe ping -attempts 5 8.8.8.8
./probe ping -timeout 2s google.com
./probe ping -loop 5s 8.8.8.8                    # Her 5 saniyede çalıştır

# TCP Probe (Port Bağlantısı)
./probe tcp example.com:22
./probe tcp google.com:443
./probe tcp -attempts 5 google.com:443
./probe tcp -loop 10s google.com:443             # Her 10 saniyede çalıştır

# HTTP/HTTPS (URL'ler)
./probe http https://google.com
./probe http -timeout 3s https://example.com
./probe http -loop 1m https://google.com         # Her 1 dakikada çalıştır

# TLS/SSL Strict Kontrol
./probe tls google.com:443
./probe tls -timeout 10s example.com:443
./probe tls -loop 30m google.com:443             # Her 30 dakikada çalıştır

# TLS/SSL Yalnızca Sertifika Kontrolü
./probe tls-cert google.com:443
./probe tls-cert https://google.com
./probe tls-cert -timeout 10s mtls.example.com:443
```

## Parametreler

```
-attempts int     Maksimum deneme sayısı (varsayılan: 3)
-timeout duration Deneme başına zaman aşımı (varsayılan: ping/tcp/http için 1s, tls/tls-cert için 5s)
-loop duration    Loop aralığı (0 = bir kez çalıştır, örn: 5s, 1m, 10s)
```

## Komutlar

```
ping               ICMP ping probe'u (IP adresleri ve hostname'leri destekler)
tcp                TCP port bağlantısı kontrolü (host:port formatı)
http               HTTP/HTTPS durum kontrolü (URL formatı)
tls                Strict TLS/SSL handshake ve sertifika doğrulaması (host:port veya URL formatı)
tls-cert           Tam handshake gerektirmeden TLS/SSL sertifika incelemesi (host:port veya URL formatı)
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
docker run --rm probe tls-cert mtls.example.com:443

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

### TLS/SSL (strict)
```
Host: google.com
Attempt: 3
Success: true
Duration: 235.67 ms
Subject: CN=www.google.com
Issuer: CN=Google Internet Authority G3
Expires At: 2027-07-28 23:59:59
Days Until Expiry: 310
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
```

### TLS/SSL (yalnızca sertifika)
```
Host: mtls.example.com
Attempt: 1
Success: true
Duration: 210.12 ms
Subject: CN=mtls.example.com
Issuer: CN=Example Issuing CA
Expires At: 2027-04-22 23:59:59
Days Until Expiry: 213
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
Handshake Warning: remote error: tls: certificate required
```

## Bağımlılıklar

| Paket | Ne için gerekli | Platform |
|---|---|---|
| `golang.org/x/net` (`icmp`, `ipv4`, `ipv6`) | ICMP ping | Yalnızca Linux |

Geri kalan her şey — TCP, HTTP ve TLS probe'ları — yalnızca Go standart
kütüphanesini kullanır. Windows'ta ICMP ping, Win32 `iphlpapi.dll` API'si
üzerinden uygulandığı için harici paket gerektirmez; diğer platformlarda ICMP
ping uygulanmamıştır. Dolayısıyla `golang.org/x/net` yalnızca Linux derlemesine
dahil olur.

`golang.org/x/sys`, `go.mod` içinde `golang.org/x/net`'in dolaylı bağımlılığı
olarak görünür.

## Gereksinimler

- Go 1.26 veya üstü (`go.mod` içinde belirtilen sürüm)
- Linux'ta ICMP Ping için:
  - Kernel desteğini kontrol edin: `cat /proc/sys/net/ipv4/ping_group_range`
  - Çekirdekte unprivileged ICMP etkinleştirilmiş olmalı
  - Etkinleştirmek için: `sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"` (systemd tabanlı çoğu dağıtımda zaten varsayılan; asıl container ve minimal imajlarda gerekir)
- TCP ve TLS probe'ları için internet bağlantısı

## Hata İşleme

Tüm probe'lar `-attempts` kadar (varsayılan 3) yeniden dener, denemeler arasında
500 ms bekler ve ilk başarıda durur. Sorun giderme için ayrıntılı hata mesajları
üretilir.

### Çıkış kodları

| Mod | Davranış |
|---|---|
| Tek seferlik | Probe başarılıysa `0`, başarısızsa `1` |
| Döngü (`-loop`) | Başarısız tur raporlanır ve döngü devam eder; süreç yalnızca `Ctrl+C` ile biter |

Geçersiz argümanlar ilk probe çalışmadan önce, `1` çıkış koduyla reddedilir:
`-attempts` en az 1 olmalı, `-timeout` ve `-loop` negatif olmamalı, bilinmeyen
komut her turda değil bir kez raporlanır.

### Gidiş-dönüş süresi nasıl ölçülür

| Platform | Kaynak |
|---|---|
| Linux | `time.Since`; sayaç gönderimden hemen önce başlatılır, cevap gelir gelmez okunur. Go'nun monotonic saatini kullanır; bu sayede duvar saati değişiklikleri — NTP sıçramaları, elle değişiklik, VM geri yükleme — sonucu bozamaz ya da negatife çeviremez. Ölçüm süreç içinde yapıldığı için gönderim ve alım sistem çağrılarının maliyetini de içerir. |
| Windows | `ICMP_ECHO_REPLY.RoundTripTime`; Win32 ICMP API'sinin kendi bildirdiği değer. Çözünürlüğü 1 ms'dir; daha hızlı cevaplarda 0 döner ve o durumda yerel olarak ölçülen süre kullanılır. |

İkisi birbirine yakındır ama birebir aynı değildir, çünkü farklı noktalardan
alınır: Linux'taki değer sistem çağrılarının etrafında ölçülür, Windows'taki
işletim sistemi tarafından üretilir. Platformlar arası karşılaştırmayı
mikrosaniye düzeyinde değil, büyüklük düzeyinde yapın.
