import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'scripts'))
import macos
import release


class BuildIdentityTests(unittest.TestCase):
    def test_single_version_and_distinct_channels(self):
        release.parse(macos.version())
        config=json.loads((ROOT/'mygo.json').read_text())
        self.assertNotIn('version',config)
        self.assertEqual(macos.identity_for('release'),('gopad','dev.knothhe.gopad'))
        self.assertEqual(macos.identity_for('dev'),macos.identity_for('preview'))
        self.assertNotEqual(macos.identity_for('release'),macos.identity_for('dev'))

    def test_missing_dev_identity_fails_before_build(self):
        with tempfile.TemporaryDirectory() as temp, mock.patch.object(macos,'LOCAL',Path(temp)/'local.json'), mock.patch.dict(os.environ,{},clear=True):
            with self.assertRaisesRegex(macos.BuildError,'fixed signing identity'):
                macos.signing('dev')
            self.assertEqual(macos.signing('preview'),('-',''))

    def test_release_cannot_fall_back_to_ad_hoc(self):
        with mock.patch.dict(os.environ,{'GOPAD_SIGNING_IDENTITY':'-'},clear=True):
            with self.assertRaisesRegex(macos.BuildError,'fixed signing identity'):
                macos.signing('release')

    def test_public_update_config_requires_trust_root(self):
        with tempfile.TemporaryDirectory() as temp, mock.patch.dict(os.environ,{},clear=True), mock.patch.object(macos,'ROOT',Path(temp)):
            (Path(temp)/'config').mkdir()
            (Path(temp)/'config/updates.json').write_text(json.dumps({'publicKey':'','github':'fixture/gopad','tagPrefix':'v','deltas':0}))
            with self.assertRaisesRegex(macos.BuildError,'public key'):
                macos.update_config()

    def test_symlink_parent_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp)
            (root/'real').mkdir()
            (root/'link').symlink_to(root/'real',target_is_directory=True)
            with self.assertRaisesRegex(macos.BuildError,'symbolic-link'):
                macos.no_symlink(root/'link'/'gopad Dev.app')

    def test_pin_schema_rejects_extra_or_invalid_values(self):
        with tempfile.TemporaryDirectory() as temp:
            pin=Path(temp)/'pin.json'
            for content in [{'schema':1,'certificateSHA256':'a'*64,'extra':1},{'schema':2,'certificateSHA256':'a'*64},{'schema':1,'certificateSHA256':'-'}]:
                pin.write_text(json.dumps(content))
                with self.assertRaises(macos.BuildError):macos.read_pin(pin)
            pin.write_text(json.dumps({'schema':1,'certificateSHA256':'a'*64}))
            self.assertEqual(macos.read_pin(pin),'a'*64)

    def test_install_rejects_unrelated_existing_app_before_build(self):
        import plistlib
        with tempfile.TemporaryDirectory() as temp:
            app=Path(temp).resolve()/'gopad Dev.app'
            (app/'Contents').mkdir(parents=True)
            (app/'Contents/Info.plist').write_bytes(plistlib.dumps({'CFBundleIdentifier':'org.other.app'}))
            with mock.patch.object(macos,'build') as build:
                with self.assertRaisesRegex(macos.BuildError,'unrelated'):
                    macos.install(app)
                build.assert_not_called()

    def test_designated_requirement_rejects_bypass_and_cdhash(self):
        identity='A'*40
        fixed='designated => identifier "dev.knothhe.gopad.dev" and certificate root = H"'+identity+'"'
        self.assertTrue(macos.stable_requirement(fixed,'dev.knothhe.gopad.dev',identity))
        self.assertFalse(macos.stable_requirement(fixed+' or true','dev.knothhe.gopad.dev',identity))
        self.assertFalse(macos.stable_requirement('designated => cdhash H"'+identity+'"','dev.knothhe.gopad.dev',identity))
        self.assertFalse(macos.stable_requirement(fixed,'org.other.app',identity))


class ReleaseEngineIntegration(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root=Path(self.temp.name)
        self.repo=self.root/'repo'
        self.repo.mkdir()
        self.call('git','init','-b','main',cwd=self.repo)
        self.call('git','config','user.name','Fixture',cwd=self.repo)
        self.call('git','config','user.email','fixture@example.invalid',cwd=self.repo)
        self.call('git','config','commit.gpgsign','false',cwd=self.repo)
        self.call('git','config','tag.gpgsign','false',cwd=self.repo)
        (self.repo/'VERSION').write_text('0.1.0\n')
        (self.repo/'.release.json').write_text(json.dumps({'version_file':'VERSION','branch':'main','remote':'origin','tag_prefix':'v','checks':[], 'release_executor':'github-actions','github_release':False,'build':[],'assets':[]}))
        self.call('git','add','VERSION','.release.json',cwd=self.repo)
        self.call('git','commit','-m','Initial fixture',cwd=self.repo)
        self.remote=self.root/'origin.git'
        self.call('git','init','--bare',str(self.remote),cwd=self.root)
        self.call('git','remote','add','origin',str(self.remote),cwd=self.repo)

    def call(self,*args,cwd=None,success=True):
        result=subprocess.run(args,cwd=cwd or self.repo,text=True,capture_output=True,env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'})
        if success:self.assertEqual(result.returncode,0,result.stderr+result.stdout)
        return result

    def engine(self,*args,success=True):
        return self.call(sys.executable,str(ROOT/'scripts/release.py'),*args,success=success)

    def test_initial_preview_and_apply_and_resume(self):
        before=self.call('git','rev-parse','HEAD').stdout
        preview=json.loads(self.engine('initial','--no-push').stdout)
        self.assertEqual(preview['release_type'],'initial')
        self.assertEqual(preview['release_executor'],'github-actions')
        self.assertEqual((self.repo/'VERSION').read_text(),'0.1.0\n')
        self.assertEqual(self.call('git','tag').stdout,'')
        self.engine('initial','--no-push','--apply')
        self.assertEqual(self.call('git','rev-parse','HEAD').stdout,before)
        tag=self.call('git','cat-file','-p','v0.1.0').stdout
        self.assertIn('Release-Type: initial',tag)
        self.engine('0.1.0','--resume','--no-push','--apply')
        self.assertEqual(self.call('git','tag').stdout.strip(),'v0.1.0')

    def test_failed_checks_never_tag_or_bump(self):
        config=json.loads((self.repo/'.release.json').read_text())
        config['checks']=[[sys.executable,'-c','raise SystemExit(1)']]
        (self.repo/'.release.json').write_text(json.dumps(config))
        self.call('git','add','.release.json')
        self.call('git','commit','-m','Configure failure')
        result=self.engine('patch','--no-push','--apply',success=False)
        self.assertNotEqual(result.returncode,0)
        self.assertEqual((self.repo/'VERSION').read_text(),'0.1.0\n')
        self.assertEqual(self.call('git','tag').stdout,'')

    def test_upgrade_requires_greater_version(self):
        result=self.engine('0.1.0','--no-push',success=False)
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(self.call('git','tag').stdout,'')

    def test_signed_initial_tag_marker_excludes_signature(self):
        signature='-----BEGIN SSH SIGNATURE-----\nfixture\n-----END SSH SIGNATURE-----'
        message=release.initial_tag_message('v0.1.0')
        with mock.patch.object(release,'git',side_effect=[message+'\n'+signature,signature]):
            self.assertEqual(release.tag_message('v0.1.0'),message)

    def test_release_notes_use_tagged_migration_instructions(self):
        self.call('git','tag','v0.1.0')
        notes=self.repo/'docs/releases/0.1.1.md'
        notes.parent.mkdir(parents=True)
        notes.write_text('Install the ZIP manually once from v0.1.0.\n')
        self.call('git','add','docs/releases/0.1.1.md')
        self.call('git','commit','-m','Document ZIP update migration')
        self.call('git','tag','v0.1.1')
        notes.write_text('Uncommitted instructions must not be published.\n')
        result=self.call(sys.executable,str(ROOT/'scripts/release-notes.py'),'fixture/gopad','v0.1.1')
        self.assertIn('Install the ZIP manually once',result.stdout)
        self.assertIn('compare/v0.1.0...v0.1.1',result.stdout)
        self.assertIn('Document ZIP update migration',result.stdout)
        self.assertNotIn('Uncommitted',result.stdout)

if __name__=='__main__':unittest.main()
