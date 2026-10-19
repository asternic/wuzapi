"""Run with python3 -m unittest discover -s tests -p '*_test.py' (requires PyYAML)."""
import json
from pathlib import Path
import re
import unittest
import yaml

ROOT = Path(__file__).resolve().parents[1]
NEW_PATHS = {
    '/chat/markplayed', '/chat/star', '/chat/pinchat', '/chat/mute',
    '/chat/unread', '/labels/edit', '/labels/chat', '/labels/message',
    '/chat/poll/vote', '/user/business-profile',
}

class DocumentationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.spec = yaml.safe_load((ROOT / 'static/api/spec.yml').read_text())
        cls.html = (ROOT / 'static/docs/index.html').read_text()

    def test_new_endpoints_match_registered_routes_and_have_examples(self):
        routes = (ROOT / 'routes.go').read_text()
        for path in NEW_PATHS:
            with self.subTest(path=path):
                self.assertIn('"' + path + '"', routes)
                operation = self.spec['paths'][path]['post']
                self.assertEqual(operation['security'], [{'ApiKeyAuth': []}])
                request = operation['requestBody']['content']['application/json']
                self.assertTrue(set(request['schema']['required']) <= request['example'].keys())
                self.assertIn('200', operation['responses'])
                self.assertIn(path, self.html)
                self.assertIn('Stdio', operation['description'])

    def test_mention_all_is_available_on_all_supported_schemas(self):
        for name in ['MessageText', 'MessageImage', 'MessageVideo', 'MessageDocument']:
            self.assertEqual(self.spec['definitions'][name]['properties']['MentionAll']['type'], 'boolean')
        edit = self.spec['paths']['/chat/send/edit']['post']['requestBody']['content']['application/json']['schema']
        self.assertEqual(edit['properties']['MentionAll']['type'], 'boolean')
        self.assertIn('ContextInfo.MentionedJID', self.html)

    def test_boolean_false_and_empty_poll_votes_are_documented(self):
        for path, boolean in [('/chat/star','Star'),('/chat/pinchat','Pin'),('/chat/mute','Mute'),('/chat/unread','Unread'),('/labels/edit','Deleted'),('/labels/chat','Labeled'),('/labels/message','Labeled')]:
            schema = self.spec['paths'][path]['post']['requestBody']['content']['application/json']['schema']
            self.assertIn(boolean,schema['required'])
            self.assertNotIn('enum',schema['properties'][boolean])
        schema = self.spec['paths']['/chat/poll/vote']['post']['requestBody']['content']['application/json']['schema']
        self.assertIn('Options',schema['required'])
        self.assertNotIn('minItems',schema['properties']['Options'])
        self.assertIn('Empty array retracts',schema['properties']['Options']['description'])

    def test_html_examples_are_valid_json_and_new_anchor_is_unique(self):
        import html
        for text in re.findall(r'<code class="language-json">(.*?)</code>',self.html,re.S):
            # Legacy docs include illustrative placeholders; verify new endpoint examples only.
            decoded=html.unescape(text)
            if '"MentionAll"' in decoded or '"LabelID"' in decoded or '"Options"' in decoded:
                json.loads(decoded)
        self.assertEqual(self.html.count('id="labels"'),1)

if __name__ == '__main__':
    unittest.main()
