# Steven

## Introduction

Some games have the concept of *events*:
things that happen at a given moment
and last for some time.
Events can also happen more than once;
in this case,
they are known as *recurring events*.
Game designers and operations can use this concept for many things,
such as:
running special campaigns;
creating seasons;
sending push notifications;
and unlocking whole features during periods of time.

This concept is implemented by an event system.
This system tracks and schedules events
and make them available for clients to query.
$GAME uses a server-authoritative, polling-based event system:
clients regularly poll an endpoint ---
say, `GET /events` ---
to retrieve the events currently running.
The current implementation computes its result on-the-fly,
i.e.,
except for deprecated events and events that can't recur anymore,
it evaluates all events whenever a client calls the endpoint.

This design is simple to understand and implement.
However, it has a negative impact in the user experience.
Some downsides are:

* Polling-based systems of this kind tend to have a high latency:
  a client needs to poll very frequently
  to make sure it doesn't stay behind;
  the latency sums up because of this,
  which, in turn, affects all other clients.
  Since all clients are doing the same thing,
  the latency goes up very quickly.
* Data might not change between requests;
  clients don't know this, though,
  so they must poll frequently.
  We waste bandwidth as a result.
  We could send a "diff" of what changed
  between the last and the current requests,
  but it would turn the system into a stateful service,
  with a bunch of edge cases
  and an overall unnecessary complexity.
* Recomputing information instead of caching it
  decreases throughput and increases response time.
  We can counterbalance these issues
  by spawning more server instances
  and by making them more powerful.
  These measures cost money, though,
  and they aren't always applicable.
* Notifying configuration changes is cumbersome.
  For instance,
  we could send clients a token,
  and make them discard their cache whenever this token changes[^dm-token];
  however,
  a configuration change doesn't always mean the whole cache is invalid.
  This results in more bandwidth waste.

[^dm-token]: In fact, this is what $GAME does.
However,
the way it triggers the discard of the cache is cumbersome,
as it *resets itself*.
The current design of the event system is not to blame here, though,
as this is a flaw of how the client was designed
to handle configuration changes.

We can do better than this.

This document is an experiment
on designing and implementing an event system
that meets the following criteria:

* The system is push-based,
  i.e., it actively notifies clients about new events.
  It should make it possible to implement incremental cache updates
  on the client side.
* The system consumes the least amount of resources possible,
  and spends most of it replying to clients.
* Listing current and next events is so fast,
  that it's indistinguishable from retrieving a list of object
  from a database over the network.

We'll call this experiment *Steven*,
an anagram of the word "events".
