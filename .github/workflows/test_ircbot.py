import contextlib
import importlib.util
import io
from pathlib import Path
import sys
import unittest
from unittest import mock


def deny_network (event, args):
  if event.startswith ("socket."):
    raise AssertionError ("IRC tests must not use real sockets: " + event)


sys.addaudithook (deny_network)


def load_bot ():
  spec = importlib.util.spec_from_file_location ("ircbot", Path (__file__).with_name ("ircbot.py"))
  bot = importlib.util.module_from_spec (spec)
  spec.loader.exec_module (bot)
  return bot


class PartialSocket:
  def __init__ (self, stalled = False):
    self.data = bytearray()
    self.timeout = None
    self.write_timeout = None
    self.stalled = stalled

  def gettimeout (self):
    return self.timeout

  def settimeout (self, value):
    self.timeout = value

  def send (self, data):
    self.write_timeout = self.timeout
    if self.stalled:
      raise TimeoutError ("mock write timed out")
    count = min (3, len (data))
    self.data.extend (data[:count])
    return count

  def sendall (self, data):
    while data:
      data = data[self.send (data):]


class BotTests (unittest.TestCase):
  def setUp (self):
    self.bot = load_bot()
    self.bot.args = self.bot.parse_args (["-j", "#test", "hello"])
    self.bot.ircsock = mock.Mock()

  def test_import_has_no_cli_or_network_side_effects (self):
    with mock.patch.object (sys, "argv", ["ircbot.py", "--invalid-option"]):
      with mock.patch ("time.sleep", side_effect = AssertionError ("unexpected sleep")):
        load_bot()

  def test_password_is_redacted_but_sent (self):
    for command in ("PASS", "pass"):
      with self.subTest (command = command):
        output = io.StringIO()
        with contextlib.redirect_stdout (output):
          self.bot.sendline (command + " dummy-password")
        self.assertNotIn ("dummy-password", output.getvalue())
        self.assertEqual (output.getvalue(), "PASS <redacted>\n")
        self.bot.ircsock.sendall.assert_called_with ((command + " dummy-password\r\n").encode())

  def test_quiet_password_has_no_log (self):
    self.bot.args.quiet = True
    output = io.StringIO()
    with contextlib.redirect_stdout (output):
      self.bot.sendline ("PASS dummy-password")
    self.assertEqual (output.getvalue(), "")

  def test_ordinary_command_is_logged (self):
    output = io.StringIO()
    with contextlib.redirect_stdout (output):
      self.bot.sendline ("NICK test")
    self.assertEqual (output.getvalue(), "NICK test\n")

  def test_partial_writes_send_complete_utf8_command (self):
    self.bot.args.quiet = True
    self.bot.ircsock = PartialSocket()
    self.bot.sendline ("PRIVMSG #test :Grüße")
    self.assertEqual (self.bot.ircsock.data, "PRIVMSG #test :Grüße\r\n".encode())
    self.assertEqual (self.bot.ircsock.write_timeout, self.bot.socket_timeout)
    self.assertIsNone (self.bot.ircsock.timeout)

  def test_stalled_write_fails_and_restores_timeout (self):
    self.bot.args.quiet = True
    self.bot.ircsock = PartialSocket (stalled = True)
    self.bot.ircsock.timeout = 7
    with self.assertRaises (TimeoutError):
      self.bot.sendline ("PING :test")
    self.assertEqual (self.bot.ircsock.write_timeout, self.bot.socket_timeout)
    self.assertEqual (self.bot.ircsock.timeout, 7)


if __name__ == "__main__":
  unittest.main()
