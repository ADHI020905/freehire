from engines import forwardable_headers


def test_identity_and_transport_headers_are_dropped():
    # The caller's User-Agent would undo the fetcher's browser impersonation, and the
    # transport-level ones are the fetcher's to set.
    got = forwardable_headers({
        "User-Agent": "freehire/0.1",
        "Host": "ex.com",
        "Content-Length": "12",
        "Accept-Encoding": "gzip",
        "Connection": "keep-alive",
        "Accept": "application/json",
        "Content-Type": "application/json",
        "X-Custom": "1",
    })
    assert got == {"Accept": "application/json", "Content-Type": "application/json", "X-Custom": "1"}


def test_result_drops_framing_headers_of_the_already_decoded_body():
    # The fetcher hands back the body decompressed and whole, so the upstream's framing
    # headers describe bytes the caller never receives.
    from engines import _result

    class Page:
        status = 200
        url = "https://ex.com/final"
        body = b"hello"
        headers = {"Content-Encoding": "gzip", "Content-Length": "999", "Transfer-Encoding": "chunked", "Content-Type": "text/html"}

    r = _result(Page())
    assert r.headers == {"Content-Type": "text/html"} and r.body == b"hello" and r.url == "https://ex.com/final"


def test_listen_address_defaults_its_host_to_loopback():
    from main import listen_address

    assert listen_address("") == ("127.0.0.1", 8098)
    assert listen_address(":9000") == ("127.0.0.1", 9000)
    assert listen_address("0.0.0.0:8098") == ("0.0.0.0", 8098)
