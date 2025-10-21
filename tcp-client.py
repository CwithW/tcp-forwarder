import requests,base64,logging
import socket


# --- Configuration ---
# The server's IP address (localhost)
HOST = '127.0.0.1'
# The port the server is listening on
PORT = 4444
# Maximum sleep time between receives
RECV_SLEEP_TIME_MAX = 0.1
# Remote URL for data transmission
REMOTE="http://example.com:8080/tcp-middleware.php"

rs = requests.Session()

logger = logging.getLogger("tcp-client")
logger.setLevel(logging.DEBUG)
logging.basicConfig(format='%(asctime)s - %(levelname)s - %(message)s')

def send_data(data: bytes) -> bool:
    """
    Sends data to the remote server via HTTP POST and returns the response data.
    """

    encoded_data = base64.b64encode(data).decode('utf-8')
    response = rs.post(REMOTE, data={"mode":"w","data":encoded_data})
    response.raise_for_status()

    response_json = response.json()
    if response_json.get('code') != 200:
        raise Exception(f"Server Error: {response_json.get('error')}")
    return True

def read_data() -> bytes:
    """
    Reads data from the remote server via HTTP POST.
    """

    response = rs.post(REMOTE, data={"mode":"r"})
    response.raise_for_status()

    response_json = response.json()
    if response_json.get('code') != 200:
        raise Exception(f"Server Error: {response_json.get('error')}")

    data = base64.b64decode(response_json.get('data', ''))
    return data


import threading
import time
class SendThread(threading.Thread):
    def __init__(self, sock: socket.socket):
        super().__init__()
        self.sock = sock
        self.stop_event = threading.Event()

    def run(self):
        try:
            while not self.stop_event.is_set():
                data = self.sock.recv(4096)
                if not data:
                    break
                try:
                    send_data(data)
                except Exception as e:
                    logger.error(f"SendThread: Error sending data: {e}")
        except Exception as e:
            logger.error(f"SendThread error: {e}")
        finally:
            self.stop_event.set()

    def stop(self):
        self.stop_event.set()

class ReceiveThread(threading.Thread):
    def __init__(self, sock: socket.socket):
        super().__init__()
        self.sock = sock
        self.stop_event = threading.Event()
        self.last_receive_time = 0

    def run(self):
        try:
            while not self.stop_event.is_set():
                data = None
                try:
                    data = read_data()
                except Exception as e:
                    logger.error(f"ReceiveThread: Error reading data: {e}")
                if not data:
                    continue
                self.sock.sendall(data)
                if self.last_receive_time - time.time() > RECV_SLEEP_TIME_MAX:
                    #sleep at most 500ms
                    should_sleep_time = self.last_receive_time + RECV_SLEEP_TIME_MAX - time.time()
                    if should_sleep_time > RECV_SLEEP_TIME_MAX:
                        should_sleep_time = RECV_SLEEP_TIME_MAX
                    time.sleep(should_sleep_time)
                    self.last_receive_time = time.time()

        except Exception as e:
            logger.error(f"ReceiveThread error: {e}")
        finally:
            self.stop_event.set()

    def stop(self):
        self.stop_event.set()





# --- Client Implementation ---
try:
    # Create a socket object
    # socket.AF_INET specifies the address family for IPv4
    # socket.SOCK_STREAM specifies the socket type for TCP
    logger.info(f"Attempting to connect to {HOST}:{PORT}...")
    client_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)

    # Connect to the server
    client_socket.connect((HOST, PORT))
    logger.info(f"Successfully connected to {HOST}:{PORT}.")

    sendThread = SendThread(client_socket)
    receiveThread = ReceiveThread(client_socket)
    sendThread.start()
    receiveThread.start()
    sendThread.join()
    receiveThread.join()
except ConnectionRefusedError:
    logger.error(f"Connection to {HOST}:{PORT} refused.")
except Exception as e:
    logger.error(f"An unexpected error occurred: {e}")
except KeyboardInterrupt as e:
    logger.info("Client is shutting down due to keyboard interrupt.")
finally:
    sendThread.stop()
    receiveThread.stop()

    client_socket.close()


print("Connection closed.")
