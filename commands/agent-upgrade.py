# Copyright 2017 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
"""
Upgrades the local agents to use the new tools binary in a directory
beside this script. Keeps all changed files in
/var/lib/juju/1.25-upgrade-rollback so that they can be restored if
needed.
"""
import json
import os
from os import path
import shutil
import sys
import tarfile
import yaml

FILE_FORMAT = '2.0'

# Config passed in from the upgrade tool.
CA_CERT = """{{.ControllerInfo.CACert}}"""
CONTROLLER_TAG = '{{.ControllerTag}}'
VERSION = '{{.Version}}'
API_ADDRESSES = """{{range .ControllerInfo.Addrs}}{{.}}
{{end}}""".splitlines()

BASE_DIR = '/var/lib/juju'
ROLLBACK_DIR = path.join(BASE_DIR, '1.25-upgrade-rollback')
TOOLS_DIR = path.join(BASE_DIR, 'tools')
AGENTS_DIR = path.join(BASE_DIR, 'agents')

UPGRADE_DIR, SCRIPT = path.split(__file__)

HOOK_TOOLS = """\
action-fail
action-get
action-set
add-metric
application-version-set
close-port
config-get
is-leader
juju-log
juju-reboot
leader-get
leader-set
network-get
opened-ports
open-port
payload-register
payload-status-set
payload-unregister
relation-get
relation-ids
relation-list
relation-set
resource-get
status-get
status-set
storage-add
storage-get
storage-list
unit-get
""".splitlines()

OLD_CONTROLLER_KEYS = """\
stateservercert
stateserverkey
caprivatekey
apiport
stateport
sharedsecret
systemidentity
""".splitlines()

# Ensure specific text is represented in literal format.
class Literal(str):
    pass

def literal_presenter(dumper, data):
    return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
yaml.add_representer(Literal, literal_presenter)


def all_agents():
    return os.listdir(AGENTS_DIR)

def save_rollback_info():
    os.mkdir(ROLLBACK_DIR)
    for agent in all_agents():
        tools_link = path.join(TOOLS_DIR, agent)
        target = os.readlink(tools_link)
        os.symlink(target, path.join(ROLLBACK_DIR, agent))

        agent_conf = path.join(AGENTS_DIR, agent, 'agent.conf')
        backup_path = path.join(ROLLBACK_DIR, agent + '_agent.conf')
        shutil.copy(agent_conf, backup_path)

def find_new_tools():
    files = [name for name in os.listdir(UPGRADE_DIR) if path.join(UPGRADE_DIR, name).endswith('.tgz')]
    assert len(files) == 1, 'too many tools files found: {}'.format(files)
    return path.join(UPGRADE_DIR, files[0])

def unpack_tools(source, dest_path):
    with tarfile.open(name=source, mode='r:gz') as contents:
        for item in contents:
            contents.extract(item, path=dest_path)
            item_path = path.join(dest_path, item.name)
            shutil.chown(item_path, 'root', 'root')

def write_tool_metadata(version, dest_path):
    with open(path.join(dest_path, 'downloaded-tools.txt'), 'w') as metadata:
        json.dump(dict(version=version, url="", size=0), metadata)

def install_tools():
    new_tools_path = find_new_tools()
    # get 2.2.3-xenial-amd64 from ~/1.25-agent-upgrade/2.2.3-xenial-amd64.tgz
    tools_base, _ = path.splitext(path.basename(new_tools_path))
    dest_path = path.join(TOOLS_DIR, tools_base)
    os.mkdir(dest_path)
    unpack_tools(new_tools_path, dest_path)
    write_tool_metadata(tools_base, dest_path)
    # Make all the hook tools link to jujud.
    make_links(dest_path, HOOK_TOOLS, path.join(dest_path, 'jujud'))
    # Make all of the agent tools dirs link to the new version.
    make_links(TOOLS_DIR, all_agents(), dest_path)

def make_links(in_dir, names, target):
    for name in names:
        link_path = path.join(in_dir, name)
        if path.exists(link_path):
            os.unlink(link_path)
        os.symlink(target, link_path)

def update_configs():
    for agent in all_agents():
        data = read_agent_config(agent)
        if agent.startswith('machine-'):
            data = update_machine_config(agent, data)
        else:
            data = update_unit_config(agent, data)
        write_agent_config(agent, data)

def config_path(agent):
    return path.join(AGENTS_DIR, agent, 'agent.conf')

def read_agent_config(agent):
    with open(config_path(agent)) as f:
        data = yaml.load(f)
    return data

def write_agent_config(agent, data):
    with open(config_path(agent), 'w') as f:
        f.write('# format %s\n' % FILE_FORMAT)
        yaml.dump(data, stream=f, default_flow_style=False)

def update_machine_config(agent, data):
    # None of these machines will need to manage the environ anymore.
    data['jobs'] = ['JobHostUnits']
    # Get rid of API/mongo hosting keys.
    for name in OLD_CONTROLLER_KEYS:
        if name in data:
            del data[name]
    return update_unit_config(agent, data)

def update_unit_config(agent, data):
    # Set controller and model.
    env_tag = data['environment']
    data['model'] = env_tag.replace('environment', 'model')
    data['controller'] = CONTROLLER_TAG

    data['upgradedToVersion'] = VERSION
    data['cacert'] = Literal(CA_CERT)

    data['apiaddresses'] = API_ADDRESSES

    # Get rid of unneeded attributes.
    for name in ('environment', 'stateaddresses', 'statepassword'):
        del data[name]

    return data

def main():
    assert not path.exists(ROLLBACK_DIR), 'saved rollback information found - aborting'
    save_rollback_info()
    install_tools()
    update_configs()

def rollback():
    assert path.exists(ROLLBACK_DIR), 'no rollback information found'
    for agent in all_agents():
        link_path = path.join(ROLLBACK_DIR, agent)
        target = os.readlink(link_path)
        dest = path.join(TOOLS_DIR, agent)
        if path.exists(dest):
            os.unlink(dest)
        os.symlink(target, dest)

        agent_conf = path.join(AGENTS_DIR, agent, 'agent.conf')
        backup_path = path.join(ROLLBACK_DIR, agent + '_agent.conf')
        shutil.copy(backup_path, agent_conf)

    tools_base, _ = path.splitext(path.basename(find_new_tools()))
    added_tools = path.join(TOOLS_DIR, tools_base)
    shutil.rmtree(added_tools)
    shutil.rmtree(ROLLBACK_DIR)

if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] == "rollback":
        rollback()
    else:
        main()
    sys.exit(0)
